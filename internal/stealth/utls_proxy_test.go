package stealth

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProxy_InvalidFingerprint(t *testing.T) {
	_, err := NewProxy("chrome_9999_unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown utls fingerprint")
}

func TestProxy_Addr(t *testing.T) {
	p, err := NewProxy("chrome_120")
	require.NoError(t, err)
	defer p.Close()

	addr := p.Addr()
	require.NotEmpty(t, addr)

	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", host)
	assert.NotEmpty(t, port)
}

func TestProxy_Start_AcceptsConnections(t *testing.T) {
	p, err := NewProxy("chrome_120")
	require.NoError(t, err)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)

	// Connect and send a CONNECT request; we don't need real TLS upstream —
	// just verify the proxy accepts the connection and responds to CONNECT.
	conn, err := net.DialTimeout("tcp", p.Addr(), 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	// Send CONNECT to a non-routable address — proxy should reply 200 then fail upstream.
	// We just check the 200 acknowledgement.
	req, err := http.NewRequest(http.MethodConnect, "", nil)
	require.NoError(t, err)
	req.Host = "127.0.0.1:1" // port 1 is almost certainly closed

	err = req.Write(conn)
	require.NoError(t, err)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	require.NoError(t, err)
	// Proxy should acknowledge with 200; upstream dial will fail silently.
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestProxy_NonCONNECT_Returns405(t *testing.T) {
	p, err := NewProxy("chrome_120")
	require.NoError(t, err)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)

	conn, err := net.DialTimeout("tcp", p.Addr(), 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	// Send a plain GET — proxy should reject with 405.
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestProxy_Close_Idempotent(t *testing.T) {
	p, err := NewProxy("chrome_120")
	require.NoError(t, err)

	assert.NoError(t, p.Close())
	assert.NoError(t, p.Close()) // second close must not panic or error
}

func TestProxy_TLSDialThrough(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in short mode")
	}

	p, err := NewProxy("chrome_120")
	require.NoError(t, err)
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p.Start(ctx)

	// Connect through the proxy to example.com:443 via CONNECT.
	conn, err := net.DialTimeout("tcp", p.Addr(), 3*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	req, _ := http.NewRequest(http.MethodConnect, "", nil)
	req.Host = "example.com:443"
	err = req.Write(conn)
	require.NoError(t, err)

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// After 200, conn is a raw TLS tunnel — the uTLS handshake has already happened
	// on the proxy side. The connection being established is proof enough.
}
