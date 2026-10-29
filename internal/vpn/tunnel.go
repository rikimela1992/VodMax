package vpn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// ErrNotConnected is returned for every dial attempted while the tunnel is
// not up. Callers must never fall back to the normal network.
var ErrNotConnected = errors.New("VPN interna non collegata")

// Tunnel is a running userspace WireGuard interface. Its DialContext opens
// connections exclusively inside the tunnel, including DNS resolution.
type Tunnel struct {
	mu     sync.RWMutex
	dev    *device.Device
	net    *netstack.Net
	closed bool
	// done is cancelled by Close so that dials in flight stop at once
	// instead of waiting for the TCP connect timeout.
	done   context.Context
	cancel context.CancelFunc
}

// Status describes the tunnel for the user interface.
type Status struct {
	Connected     bool      `json:"connected"`
	LastHandshake time.Time `json:"last_handshake,omitempty"`
	RxBytes       int64     `json:"rx_bytes"`
	TxBytes       int64     `json:"tx_bytes"`
}

// handshakeFreshness mirrors WireGuard's reject-after time: without a
// handshake in this window no data can flow.
const handshakeFreshness = 180 * time.Second

// Resolver looks up the VPN server's own address. It is the only name
// resolved on the normal network, and it is never a provider host.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Start brings the tunnel up. The endpoint is resolved with resolver
// (net.DefaultResolver when nil).
func Start(ctx context.Context, cfg *Config, resolver Resolver) (*Tunnel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	endpoint, err := resolveEndpoint(ctx, cfg.Endpoint, resolver)
	if err != nil {
		return nil, err
	}
	local := make([]netip.Addr, 0, len(cfg.Addresses))
	for _, p := range cfg.Addresses {
		local = append(local, p.Addr())
	}
	tunDev, tnet, err := netstack.CreateNetTUN(local, cfg.DNS, cfg.mtu())
	if err != nil {
		return nil, fmt.Errorf("creazione stack di rete: %w", err)
	}
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	if err := dev.IpcSet(cfg.uapi(endpoint)); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configurazione WireGuard: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("avvio WireGuard: %w", err)
	}
	done, cancel := context.WithCancel(context.Background())
	return &Tunnel{dev: dev, net: tnet, done: done, cancel: cancel}, nil
}

func resolveEndpoint(ctx context.Context, endpoint string, resolver Resolver) (netip.AddrPort, error) {
	host, port, err := splitHostPort(endpoint)
	if err != nil {
		return netip.AddrPort{}, err
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return netip.AddrPortFrom(addr, uint16(port)), nil
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return netip.AddrPort{}, fmt.Errorf("server VPN %q non trovato", host)
	}
	return netip.AddrPortFrom(addrs[0].Unmap(), uint16(port)), nil
}

// DialContext connects through the tunnel. Host names are resolved by the
// DNS servers of the VPN configuration, inside the tunnel.
func (t *Tunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if t == nil {
		return nil, ErrNotConnected
	}
	t.mu.RLock()
	closed, tnet := t.closed, t.net
	t.mu.RUnlock()
	if closed {
		return nil, ErrNotConnected
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	defer context.AfterFunc(t.done, stop)()
	c, err := tnet.DialContext(ctx, network, address)
	if err != nil && t.done.Err() != nil {
		return nil, ErrNotConnected
	}
	return c, err
}

// Up reports whether the tunnel is running. Traffic may still be stalled
// until a handshake completes; see Status.
func (t *Tunnel) Up() bool {
	if t == nil {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return !t.closed
}

// Status reads counters and handshake time from the WireGuard device.
func (t *Tunnel) Status() Status {
	if !t.Up() {
		return Status{}
	}
	t.mu.RLock()
	raw, err := t.dev.IpcGet()
	t.mu.RUnlock()
	if err != nil {
		return Status{}
	}
	var s Status
	var sec, nsec int64
	for _, line := range strings.Split(raw, "\n") {
		key, value, _ := strings.Cut(line, "=")
		n, _ := strconv.ParseInt(value, 10, 64)
		switch key {
		case "last_handshake_time_sec":
			sec = n
		case "last_handshake_time_nsec":
			nsec = n
		case "rx_bytes":
			s.RxBytes = n
		case "tx_bytes":
			s.TxBytes = n
		}
	}
	if sec > 0 {
		s.LastHandshake = time.Unix(sec, nsec)
		s.Connected = time.Since(s.LastHandshake) < handshakeFreshness
	}
	return s
}

// Close tears the tunnel down; subsequent dials fail with ErrNotConnected.
func (t *Tunnel) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	t.cancel()
	t.dev.Close()
}
