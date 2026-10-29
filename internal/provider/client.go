// Package provider builds the only HTTP client allowed to talk to IPTV
// providers. It dials exclusively through the internal VPN.
package provider

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/rikimela1992/vodmax/internal/vpn"
)

// Dialer is satisfied by *vpn.Tunnel. Nothing else should implement it
// outside tests.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	Up() bool
}

// NewHTTPClient returns a client whose every connection goes through d.
// There is deliberately no option to use the normal network or a proxy.
func NewHTTPClient(d Dialer, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		// Environment proxies (HTTP_PROXY, ...) would bypass the tunnel.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if d == nil || !d.Up() {
				return nil, vpn.ErrNotConnected
			}
			return d.DialContext(ctx, network, address)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

// Redact strips the query string and user info from URLs in err, since
// provider URLs carry credentials (Xtream puts the password in the query
// and in the path, so the path is dropped too).
func Redact(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		safe := "(url del provider)"
		if u, perr := url.Parse(uerr.URL); perr == nil {
			safe = u.Scheme + "://" + u.Host + "/…"
		}
		return &url.Error{Op: uerr.Op, URL: safe, Err: uerr.Err}
	}
	return err
}
