package vpn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func testKeyPair(t *testing.T) (Key, Key) {
	t.Helper()
	var priv Key
	if _, err := rand.Read(priv[:]); err != nil {
		t.Fatal(err)
	}
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	var k Key
	copy(k[:], pub)
	return priv, k
}

func b64(k Key) string { return base64.StdEncoding.EncodeToString(k[:]) }

// fakeProvider is the far side of the tunnel: a WireGuard peer on
// 127.0.0.1 whose own netstack serves HTTP on 100.64.0.1:80, as a provider
// reachable only through the VPN would.
type fakeProvider struct {
	dev  *device.Device
	port int
	pub  Key
}

func startFakeProvider(t *testing.T, clientPub Key) *fakeProvider {
	t.Helper()
	priv, pub := testKeyPair(t)
	tunDev, tnet, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("100.64.0.1")}, nil, 1420)
	if err != nil {
		t.Fatal(err)
	}
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	uapi := fmt.Sprintf("private_key=%s\nlisten_port=%d\npublic_key=%s\nallowed_ip=100.64.0.2/32\n",
		priv.hex(), port, clientPub.hex())
	if err := dev.IpcSet(uapi); err != nil {
		t.Fatal(err)
	}
	if err := dev.Up(); err != nil {
		t.Fatal(err)
	}
	listener, err := tnet.ListenTCP(&net.TCPAddr{Port: 80})
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "playlist via tunnel")
	})}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close(); dev.Close() })
	return &fakeProvider{dev: dev, port: port, pub: pub}
}

func clientConfig(t *testing.T) (*Config, Key) {
	priv, pub := testKeyPair(t)
	return &Config{
		PrivateKey: priv,
		Addresses:  []netip.Prefix{netip.MustParsePrefix("100.64.0.2/32")},
		DNS:        []netip.Addr{netip.MustParseAddr("100.64.0.1")},
		AllowedIPs: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
	}, pub
}

func tunnelHTTP(tun *Tunnel) *http.Client {
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: tun.DialContext}}
}

func TestTunnelEndToEnd(t *testing.T) {
	cfg, clientPub := clientConfig(t)
	provider := startFakeProvider(t, clientPub)
	cfg.PeerPublicKey = provider.pub
	cfg.Endpoint = fmt.Sprintf("127.0.0.1:%d", provider.port)

	tun, err := Start(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()

	resp, err := tunnelHTTP(tun).Get("http://100.64.0.1/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "playlist via tunnel" {
		t.Fatalf("unexpected body %q", body)
	}
	if st := tun.Status(); !st.Connected || st.TxBytes == 0 || st.LastHandshake.IsZero() {
		t.Fatalf("status after traffic: %+v", st)
	}

	// The provider address does not exist on the normal network: the only
	// way the request above could succeed is through the tunnel.
	if c, err := (&net.Dialer{Timeout: 300 * time.Millisecond}).Dial("tcp", "100.64.0.1:80"); err == nil {
		c.Close()
		t.Fatal("provider reachable outside the tunnel: test setup is invalid")
	}
}

func TestClosedTunnelNeverDials(t *testing.T) {
	cfg, clientPub := clientConfig(t)
	provider := startFakeProvider(t, clientPub)
	cfg.PeerPublicKey = provider.pub
	cfg.Endpoint = fmt.Sprintf("127.0.0.1:%d", provider.port)
	tun, err := Start(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	tun.Close()
	if tun.Up() || tun.Status().Connected {
		t.Fatal("closed tunnel reported as up")
	}
	if _, err := tun.DialContext(context.Background(), "tcp", "100.64.0.1:80"); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
	var none *Tunnel
	if _, err := none.DialContext(context.Background(), "tcp", "100.64.0.1:80"); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("nil tunnel: expected ErrNotConnected, got %v", err)
	}
}

func TestUnreachablePeerDoesNotLeak(t *testing.T) {
	// A tunnel whose peer never answers must time out inside the tunnel,
	// never reaching the destination by other means.
	cfg, _ := clientConfig(t)
	_, cfg.PeerPublicKey = testKeyPair(t)
	cfg.Endpoint = "127.0.0.1:9" // discard port: no WireGuard there
	tun, err := Start(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()

	direct := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached the normal network")
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go direct.Serve(listener)
	defer direct.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+listener.Addr().String()+"/", nil)
	if _, err := tunnelHTTP(tun).Do(req); err == nil {
		t.Fatal("request succeeded without a working tunnel")
	}
	if tun.Status().Connected {
		t.Fatal("tunnel without handshake reported as connected")
	}
}

func TestCloseInterruptsDialsInFlight(t *testing.T) {
	// Regression: Close used to wait for a pending dial to hit the TCP
	// connect timeout (~2 minutes) when the VPN peer did not answer.
	cfg, _ := clientConfig(t)
	_, cfg.PeerPublicKey = testKeyPair(t)
	cfg.Endpoint = "127.0.0.1:9"
	tun, err := Start(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := tun.DialContext(context.Background(), "tcp", "100.64.0.1:80")
		result <- err
	}()
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	tun.Close()
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Close blocked for %v", d)
	}
	select {
	case err := <-result:
		if !errors.Is(err, ErrNotConnected) {
			t.Fatalf("pending dial: expected ErrNotConnected, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending dial not interrupted by Close")
	}
}
