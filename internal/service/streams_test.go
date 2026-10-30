package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/silirim/vodmax/internal/secret"
	"github.com/silirim/vodmax/internal/store"
)

// A download holds the only connection until someone wants to watch: then
// it gives it up at once, and cannot take it back while they watch.
func TestBackgroundGivesWay(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "vodmax.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	box, _ := secret.LoadOrCreate(filepath.Join(dir, "secret.key"))
	s := New(st, box, nil)
	vpnID, _ := st.AddVPN(ctx, "v", "system", func(int64) []byte { return []byte("x") })
	sub, _ := st.AddSubscription(ctx, store.Subscription{Name: "B", Kind: "xtream", VPNID: vpnID, Settings: json.RawMessage(`{}`)},
		func(int64) []byte { return []byte("x") })
	st.SetSyncResult(ctx, sub, "ok", "", json.RawMessage(`{"max_connections":1}`))

	bg, release, err := s.acquireBackground(ctx, sub, "download:1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.acquireBackground(ctx, sub, "download:2"); !errors.As(err, new(ConnectionBusy)) {
		t.Fatalf("second download: %v", err)
	}
	go func() { <-bg.Done(); release() }()
	_, watch, err := s.acquire(ctx, sub, "salotto-tv-01", false, false)
	if err != nil {
		t.Fatalf("viewer refused because of a download: %v", err)
	}
	select {
	case <-bg.Done():
	case <-time.After(time.Second):
		t.Fatal("the download kept the connection")
	}
	if _, _, err := s.acquireBackground(ctx, sub, "download:1"); !errors.As(err, new(ConnectionBusy)) {
		t.Fatalf("download took the connection back: %v", err)
	}
	watch()
}
