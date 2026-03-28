package stealth

import (
	"bufio"
	"context"
	stdtls "crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
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
	ca          *CA    // For MITM mode: dynamic cert generation
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

// NewMITMProxy starts a CONNECT proxy that performs true TLS fingerprint
// spoofing. It terminates TLS from Chromium with a dynamically-generated cert,
// then re-establishes the upstream connection using uTLS with the target
// ClientHello fingerprint.
func NewMITMProxy(fingerprint string) (*Proxy, error) {
	id, err := ResolveFingerprint(fingerprint)
	if err != nil {
		return nil, fmt.Errorf("utls mitm proxy: %w", err)
	}

	ca, err := NewCA()
	if err != nil {
		return nil, fmt.Errorf("utls mitm proxy: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("utls mitm proxy: listen: %w", err)
	}

	p := &Proxy{
		listener:    ln,
		fingerprint: fingerprint,
		helloID:     id,
		ca:          ca,
		mode:        "mitm",
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

// handleConn dispatches to relay or MITM mode based on the proxy configuration.
func (p *Proxy) handleConn(ctx context.Context, clientConn net.Conn) {
	if p.mode == "mitm" {
		p.handleMITM(ctx, clientConn)
	} else {
		p.handleRelay(ctx, clientConn)
	}
}

// handleRelay processes a CONNECT request and creates a transparent TCP tunnel.
// Chromium's TLS goes directly to the upstream server unchanged.
func (p *Proxy) handleRelay(_ context.Context, clientConn net.Conn) {
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
	pipeConns(clientConn, upstream)
}

// handleMITM performs a true MITM TLS interception:
//  1. Read CONNECT from Chromium, extract host
//  2. Dial upstream TCP
//  3. Send 200 to Chromium
//  4. Perform uTLS handshake with upstream (spoofed fingerprint)
//  5. Generate dynamic cert for host via CA
//  6. Perform standard TLS handshake with Chromium (as server)
//  7. Bidirectional pipe between the two TLS connections
func (p *Proxy) handleMITM(_ context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	br := bufio.NewReader(clientConn)
	req, err := http.ReadRequest(br)
	if err != nil {
		slog.Debug("utls proxy mitm: failed to read request", "error", err)
		return
	}

	if req.Method != http.MethodConnect {
		slog.Debug("utls proxy mitm: non-CONNECT request", "method", req.Method)
		writeHTTPError(clientConn, http.StatusMethodNotAllowed, "only CONNECT supported")
		return
	}

	host := req.Host
	if host == "" {
		writeHTTPError(clientConn, http.StatusBadRequest, "missing host")
		return
	}

	// Extract the hostname without port for certificate generation.
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	// Ensure host has a port for dialing.
	dialHost := host
	if !strings.Contains(host, ":") {
		dialHost = host + ":443"
	}

	// Step 1: Dial upstream with raw TCP.
	rawUpstream, err := net.DialTimeout("tcp", dialHost, 10*time.Second)
	if err != nil {
		slog.Warn("utls proxy mitm: upstream dial failed", "host", host, "error", err)
		writeHTTPError(clientConn, http.StatusBadGateway, "upstream unreachable")
		return
	}
	defer rawUpstream.Close()

	// Step 2: Send 200 to Chromium before starting TLS.
	if _, err := fmt.Fprintf(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		slog.Debug("utls proxy mitm: failed to send 200", "error", err)
		return
	}

	// Drain any buffered bytes into clientConn's underlying connection.
	// After the 200, Chromium will start a TLS handshake, so we need a raw conn.
	var rawClient net.Conn
	if br.Buffered() > 0 {
		rawClient = &prefixConn{Reader: br, Conn: clientConn}
	} else {
		rawClient = clientConn
	}

	// Step 3: uTLS handshake with upstream (spoofed fingerprint).
	utlsConn := tls.UClient(rawUpstream, &tls.Config{
		ServerName:         hostname,
		InsecureSkipVerify: true, //nolint:gosec // MITM proxy intentionally skips upstream cert verification
		NextProtos:         []string{"http/1.1"}, // Force HTTP/1.1 — no h2 initially
	}, p.helloID)
	if err := utlsConn.Handshake(); err != nil {
		slog.Warn("utls proxy mitm: upstream TLS handshake failed", "host", host, "error", err)
		return
	}
	defer utlsConn.Close()

	// Step 4: Generate a dynamic certificate for this host.
	cert, err := p.ca.IssueCert(hostname)
	if err != nil {
		slog.Warn("utls proxy mitm: cert generation failed", "host", hostname, "error", err)
		return
	}

	// Step 5: Standard TLS handshake with Chromium (proxy acts as server).
	// Use crypto/tls (NOT utls) for the server side.
	tlsServer := stdtls.Server(rawClient, &stdtls.Config{
		Certificates: []stdtls.Certificate{*cert},
		NextProtos:   []string{"http/1.1"},
	})
	if err := tlsServer.Handshake(); err != nil {
		slog.Debug("utls proxy mitm: client TLS handshake failed", "host", host, "error", err)
		return
	}
	defer tlsServer.Close()

	slog.Debug("utls proxy mitm: tunnel open", "host", host, "fingerprint", p.fingerprint)

	// Step 6: Bidirectional pipe between Chromium TLS and upstream uTLS.
	pipeConns(tlsServer, utlsConn)
}

// pipeConns does a bidirectional copy between two connections.
func pipeConns(a, b net.Conn) {
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		done <- struct{}{}
	}
	go pipe(a, b)
	go pipe(b, a)
	<-done
	<-done
}

// prefixConn wraps a buffered reader with a net.Conn so that any bytes already
// buffered by the reader are consumed before reading from the underlying conn.
type prefixConn struct {
	io.Reader
	net.Conn
}

func (c *prefixConn) Read(b []byte) (int, error) {
	return c.Reader.Read(b)
}

func writeHTTPError(conn net.Conn, code int, msg string) {
	resp := fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: %d\r\n\r\n%s",
		code, http.StatusText(code), len(msg), msg)
	_, _ = conn.Write([]byte(resp))
}
