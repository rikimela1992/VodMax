package service

import (
	"context"
	"net"
	"time"

	"github.com/rikimela1992/vodmax/internal/vpn"
)

// systemNetwork is the "VPN" of kind system: the user runs a VPN outside
// VodMax (on the device, or on the router for the whole network), so the
// provider is reached through the normal network. VodMax cannot verify
// that this traffic really goes through a VPN.
type systemNetwork struct{}

func (systemNetwork) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	return d.DialContext(ctx, network, address)
}

func (systemNetwork) Up() bool { return true }

// Status reports it as connected: there is no tunnel to watch.
func (systemNetwork) Status() vpn.Status { return vpn.Status{Connected: true} }

func (systemNetwork) Close() {}
