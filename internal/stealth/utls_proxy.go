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

// Proxy is a CONNECT-based forward proxy that dials upstream TLS connections
// using a configurable uTLS ClientHello fingerprint.
// It is safe for concurrent use.
type Proxy struct {
	listener    net.Listener
	fingerprint string
	helloID     tls.ClientHelloID
	mu          sync.Mutex
	closed      bool
}

// NewProxy starts a CONNECT proxy listening on a random localhost port.
// fingerprint must be a key known to ResolveFingerprint; an empty string
// falls back to DefaultFingerprint.
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
	}
	return p, nil
}

// Addr returns the address the proxy is listening on (e.g. "127.0.0.1:54321").
func (p *Proxy) Addr() string {
	return p.listener.Addr().String()
}

// Start begins accepting connections in a background goroutine.
// The proxy runs until Close is called or ctx is cancelled.
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

// acceptLoop accepts connections until the listener is closed.
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
			// Back off briefly to avoid spinning on persistent errors.
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

// handleConn processes a single CONNECT request and pipes traffic.
func (p *Proxy) handleConn(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	br := bufio.NewReader(clientConn)
	req, err := http.ReadRequest(br)
	if err != nil {
		slog.Debug("utls proxy: failed to read request", "error", err)
		return
	}

	if req.Method != http.MethodConnect {
		slog.Debug("utls proxy: non-CONNECT request", "method", req.Method, "url", req.URL)
		writeHTTPError(clientConn, http.StatusMethodNotAllowed, "only CONNECT supported")
		return
	}

	host := req.Host
	if host == "" {
		writeHTTPError(clientConn, http.StatusBadRequest, "missing host")
		return
	}

	// Acknowledge the tunnel.
	if _, err := fmt.Fprintf(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		slog.Debug("utls proxy: failed to send 200", "error", err)
		return
	}

	// Dial upstream with uTLS.
	upstream, err := p.dialTLS(ctx, host)
	if err != nil {
		slog.Warn("utls proxy: upstream dial failed", "host", host, "error", err)
		return
	}
	defer upstream.Close()

	slog.Debug("utls proxy: tunnel open", "host", host, "fingerprint", p.fingerprint)

	// Bidirectional pipe — errors are informational only.
	done := make(chan struct{}, 2)
	copyAndSignal := func(dst io.Writer, src io.Reader) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go copyAndSignal(upstream, clientConn)
	go copyAndSignal(clientConn, upstream)

	// Wait for either direction to finish or context to cancel.
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// dialTLS opens a uTLS connection to host (host:port) using the configured fingerprint.
func (p *Proxy) dialTLS(ctx context.Context, host string) (*tls.UConn, error) {
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		// host without port — treat as hostname only (shouldn't normally happen via CONNECT)
		hostname = host
		host = net.JoinHostPort(host, "443")
	}

	rawConn, err := (&net.Dialer{}).DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, fmt.Errorf("tcp dial %s: %w", host, err)
	}

	tlsConn := tls.UClient(rawConn, &tls.Config{
		ServerName: hostname,
	}, p.helloID)

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("tls handshake %s: %w", hostname, err)
	}

	return tlsConn, nil
}

// writeHTTPError sends a plain HTTP error response on conn.
func writeHTTPError(conn net.Conn, code int, msg string) {
	resp := fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: %d\r\n\r\n%s",
		code, http.StatusText(code), len(msg), msg)
	_, _ = conn.Write([]byte(resp))
}
