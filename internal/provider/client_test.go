package provider

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/silirim/vodmax/internal/vpn"
)

type downDialer struct{ dials atomic.Int32 }

func (d *downDialer) Up() bool { return false }
func (d *downDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d.dials.Add(1)
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

func TestNoTrafficWithoutVPN(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()

	d := &downDialer{}
	for _, client := range []*http.Client{NewHTTPClient(d, 5*time.Second), NewHTTPClient(nil, 5*time.Second)} {
		_, err := client.Get(srv.URL + "/player_api.php?username=u&password=secret")
		if !errors.Is(err, vpn.ErrNotConnected) {
			t.Fatalf("expected ErrNotConnected, got %v", err)
		}
	}
	if hits.Load() != 0 || d.dials.Load() != 0 {
		t.Fatalf("traffic left without VPN: hits=%d dials=%d", hits.Load(), d.dials.Load())
	}
}

func TestEnvironmentProxyIgnored(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	tr := NewHTTPClient(&downDialer{}, time.Second).Transport.(*http.Transport)
	if tr.Proxy != nil {
		t.Fatal("transport must not use environment proxies")
	}
}

func TestRedactHidesCredentials(t *testing.T) {
	client := NewHTTPClient(nil, time.Second)
	_, err := client.Get("http://provider.example/live/user/secret/1.ts?password=secret")
	err = Redact(err)
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "user") {
		t.Fatalf("credentials visible in error: %v", err)
	}
	if !errors.Is(err, vpn.ErrNotConnected) {
		t.Fatalf("cause lost: %v", err)
	}
}
