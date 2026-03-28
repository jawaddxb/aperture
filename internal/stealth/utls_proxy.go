package stealth

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	tls "github.com/refraction-networking/utls"
)

// Proxy is a CONNECT-based forward proxy that re-wraps the upstream TLS
// connection with a uTLS ClientHello fingerprint.
//
// Architecture: Chromium → CONNECT → Proxy → uTLS handshake → upstream
//
// The proxy terminates Chromium's inner TLS and re-establishes it upstream
// using uTLS with the configured fingerprint. This is a MITM approach where:
// 1. Chromium sends CONNECT host:443
// 2. Proxy sends 200 Connection Established
// 3. Chromium starts TLS with proxy (proxy acts as the server)
// 4. Proxy starts uTLS with upstream (proxy acts as client with spoofed fingerprint)
// 5. Data is piped between the two TLS connections
//
// For this to work with Chromium's --proxy-server flag, we use a simpler
// approach: plain TCP tunnel where the proxy just relays bytes but
// intercepts at the TCP level before TLS begins.
//
// CORRECT APPROACH: The proxy opens a raw TCP connection upstream, then
// acts as a transparent relay. Chromium's TLS ClientHello goes directly
// to the upstream server. To spoof the fingerprint, we need to intercept
// the ClientHello bytes and rewrite them — which uTLS doesn't support
// for relay mode.
//
// PRACTICAL APPROACH: Use uTLS in client mode. The proxy:
// 1. Accepts CONNECT from Chromium
// 2. Sends 200
// 3. Chromium sends TLS ClientHello to proxy (thinking it's the server)
// 4. Proxy completes TLS handshake with Chromium using a self-signed cert
// 5. Proxy connects to upstream with uTLS (clean fingerprint)
// 6. Pipes decrypted data between the two TLS connections
//
// This requires generating a CA cert that Chromium trusts.
// Since we control the Chromium launch args, we can add --ignore-certificate-errors.
//
// SIMPLEST CORRECT APPROACH: Just do a plain TCP relay (no uTLS in the proxy).
// Instead, set the uTLS fingerprint via the Chromium --proxy-server and use
// environment flags. BUT Chromium doesn't support uTLS natively.
//
// ACTUALLY CORRECT: Use a simple CONNECT tunnel that does a raw TCP relay
// (no TLS termination) to the upstream. The TLS fingerprint is then
// Chromium's native one. For uTLS fingerprint spoofing, we need the
// MITM approach with a local CA.
type Proxy struct {
	listener    net.Listener
	fingerprint string
	helloID     tls.ClientHelloID
	caCert      *tls.Certificate // For MITM: sign certs on the fly
	mu          sync.Mutex
	closed      bool
	mode        string // "relay" (transparent) or "mitm" (fingerprint spoof)
}

// NewProxy starts a CONNECT proxy on a random localhost port.
// In "relay" mode (default), it's a transparent TCP tunnel — no TLS modification.
// This is useful for routing but does NOT spoof TLS fingerprints.
// True uTLS fingerprint spoofing requires MITM mode (future).
func NewProxy(fingerprint string) (*Proxy, error) {
	id, err := ResolveFingerprint(fingerprint)
	if err != nil {
		return nil, fmt.Errorf("utls proxy: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("utls proxy: listen: %w", err)
	}

	p := &Proxy{
		listener:    ln,
		fingerprint: fingerprint,
		helloID:     id,
		mode:        "relay",
	}
	return p, nil
}

// Addr returns the address the proxy is listening on.
func (p *Proxy) Addr() string {
	return p.listener.Addr().String()
}

// Start begins accepting connections in a background goroutine.
func (p *Proxy) Start(ctx context.Context) {
	go p.acceptLoop(ctx)
}

// Close shuts down the proxy listener.
func (p *Proxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return p.listener.Close()
}

func (p *Proxy) acceptLoop(ctx context.Context) {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			p.mu.Lock()
			closed := p.closed
			p.mu.Unlock()
			if closed {
				return
			}
			slog.Warn("utls proxy: accept error", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}
		go p.handleConn(ctx, conn)
	}
}

// handleConn processes a CONNECT request and creates a transparent TCP tunnel.
// In relay mode, Chromium's TLS goes directly to the upstream server unchanged.
func (p *Proxy) handleConn(_ context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	br := bufio.NewReader(clientConn)
	req, err := http.ReadRequest(br)
	if err != nil {
		slog.Debug("utls proxy: failed to read request", "error", err)
		return
	}

	if req.Method != http.MethodConnect {
		slog.Debug("utls proxy: non-CONNECT request", "method", req.Method)
		writeHTTPError(clientConn, http.StatusMethodNotAllowed, "only CONNECT supported")
		return
	}

	host := req.Host
	if host == "" {
		writeHTTPError(clientConn, http.StatusBadRequest, "missing host")
		return
	}

	// Connect to the upstream server with raw TCP.
	upstream, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		slog.Warn("utls proxy: upstream dial failed", "host", host, "error", err)
		writeHTTPError(clientConn, http.StatusBadGateway, "upstream unreachable")
		return
	}
	defer upstream.Close()

	// Send 200 to tell Chromium the tunnel is ready.
	if _, err := fmt.Fprintf(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		slog.Debug("utls proxy: failed to send 200", "error", err)
		return
	}

	// If there are buffered bytes from the reader, write them to upstream first.
	if br.Buffered() > 0 {
		buffered := make([]byte, br.Buffered())
		n, _ := br.Read(buffered)
		if n > 0 {
			if _, err := upstream.Write(buffered[:n]); err != nil {
				return
			}
		}
	}

	slog.Debug("utls proxy: tunnel open", "host", host, "mode", p.mode)

	// Bidirectional raw TCP relay.
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		// Signal completion and half-close.
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		done <- struct{}{}
	}

	go pipe(upstream, clientConn)
	go pipe(clientConn, upstream)

	// Wait for both directions to finish.
	<-done
	<-done
}

func writeHTTPError(conn net.Conn, code int, msg string) {
	resp := fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: %d\r\n\r\n%s",
		code, http.StatusText(code), len(msg), msg)
	_, _ = conn.Write([]byte(resp))
}
