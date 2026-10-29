package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rikimela1992/vodmax/internal/secret"
	"github.com/rikimela1992/vodmax/internal/store"
	"github.com/rikimela1992/vodmax/internal/vpn"
)

func TestCatalogDue(t *testing.T) {
	loc := time.Local
	at := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, loc) }
	ptr := func(t time.Time) *time.Time { return &t }
	sc := Schedule{CatalogHour: 4}
	cases := []struct {
		name   string
		synced *time.Time
		status string
		now    time.Time
		want   bool
	}{
		{"never synced", nil, "", at(29, 15, 0), true},
		{"synced after tonight's slot", ptr(at(29, 4, 10)), "ok", at(29, 15, 0), false},
		{"synced before tonight's slot", ptr(at(29, 3, 50)), "ok", at(29, 4, 1), true},
		{"before the slot, synced yesterday after it", ptr(at(28, 4, 5)), "ok", at(29, 3, 0), false},
		{"missed a night", ptr(at(27, 12, 0)), "ok", at(29, 3, 0), true},
		{"failed recently", ptr(at(29, 4, 1)), "error", at(29, 4, 30), false},
		{"failed an hour ago", ptr(at(29, 4, 1)), "error", at(29, 5, 2), true},
	}
	for _, c := range cases {
		if got := catalogDue(sc, c.synced, c.status, c.now); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
	if catalogDue(Schedule{CatalogHour: -1}, nil, "", at(29, 4, 0)) {
		t.Error("disabled schedule still syncs")
	}
}

type stubTunnel struct {
	up, connected atomic.Bool
}

func (f *stubTunnel) Up() bool { return f.up.Load() }
func (f *stubTunnel) Status() vpn.Status {
	return vpn.Status{Connected: f.up.Load() && f.connected.Load()}
}
func (f *stubTunnel) Close() { f.up.Store(false) }
func (f *stubTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return nil, fmt.Errorf("offline")
}

func TestVPNWatchdog(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	box, err := secret.LoadOrCreate(filepath.Join(dir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "vodmax.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var starts atomic.Int32
	var fail atomic.Bool
	var current atomic.Pointer[stubTunnel]
	svc := New(st, box, func(ctx context.Context, cfg *vpn.Config) (Tunnel, error) {
		starts.Add(1)
		if fail.Load() {
			return nil, fmt.Errorf("endpoint irraggiungibile")
		}
		tn := &stubTunnel{}
		tn.up.Store(true)
		tn.connected.Store(true)
		current.Store(tn)
		return tn, nil
	})
	defer svc.Close()
	key := make([]byte, 32)
	rand.Read(key)
	k := base64.StdEncoding.EncodeToString(key)
	conf := fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = 10.2.0.2/32\nDNS = 10.2.0.1\n[Peer]\nPublicKey = %s\nAllowedIPs = 0.0.0.0/0\nEndpoint = 127.0.0.1:51820\n", k, k)
	vpnID, err := svc.AddVPN(ctx, "Prova", conf)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// A VPN no subscription uses is left alone.
	svc.checkVPNs(ctx, log)
	if starts.Load() != 0 {
		t.Fatal("unused VPN started")
	}
	if _, err := svc.AddSubscription(ctx, SubscriptionInput{Name: "A", Kind: "m3u", VPNID: vpnID, URL: "http://example.invalid/list.m3u"}); err != nil {
		t.Fatal(err)
	}
	svc.checkVPNs(ctx, log)
	if starts.Load() != 1 {
		t.Fatalf("starts = %d, want 1", starts.Load())
	}
	svc.checkVPNs(ctx, log)
	if v := health(t, svc, vpnID); !v.OK {
		t.Fatalf("health after connect: %+v", v)
	}

	// The tunnel dies: restarted at the next check.
	current.Load().Close()
	svc.checkVPNs(ctx, log)
	if starts.Load() != 2 {
		t.Fatalf("dead tunnel not restarted (starts = %d)", starts.Load())
	}

	// No handshake: restarted only after vpnStaleAfter.
	current.Load().connected.Store(false)
	svc.checkVPNs(ctx, log)
	svc.mu.Lock()
	svc.maint.vpn[vpnID].stale = time.Now().Add(-vpnStaleAfter - time.Second)
	svc.mu.Unlock()
	fail.Store(true)
	svc.checkVPNs(ctx, log)
	if starts.Load() != 3 {
		t.Fatalf("stale tunnel not restarted (starts = %d)", starts.Load())
	}
	v := health(t, svc, vpnID)
	if v.OK || !strings.Contains(v.Error, "endpoint irraggiungibile") {
		t.Fatalf("health after failure: %+v", v)
	}
	// Back-off: no immediate retry.
	svc.checkVPNs(ctx, log)
	if starts.Load() != 3 {
		t.Fatal("retried without waiting")
	}
	status, err := svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Problems) == 0 || status.Problems[0].Area != "vpn" {
		t.Fatalf("problems: %+v", status.Problems)
	}

	// Disconnected by the user: not reconnected.
	fail.Store(false)
	svc.DisconnectVPN(vpnID)
	svc.mu.Lock()
	svc.maint.vpn[vpnID].next = time.Time{}
	svc.mu.Unlock()
	svc.checkVPNs(ctx, log)
	if starts.Load() != 3 {
		t.Fatal("reconnected a VPN the user disconnected")
	}
}

func health(t *testing.T, svc *Service, id int64) VPNHealth {
	t.Helper()
	list, err := svc.VPNs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range list {
		if v.ID == id && v.Health != nil {
			return *v.Health
		}
	}
	t.Fatalf("no health for VPN %d", id)
	return VPNHealth{}
}
