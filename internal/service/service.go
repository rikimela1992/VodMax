// Package service ties together storage, encrypted secrets, the internal
// VPN tunnels and catalog synchronisation.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rikimela1992/vodmax/internal/catalog"
	"github.com/rikimela1992/vodmax/internal/playlist/m3u"
	"github.com/rikimela1992/vodmax/internal/playlist/xtream"
	"github.com/rikimela1992/vodmax/internal/provider"
	"github.com/rikimela1992/vodmax/internal/secret"
	"github.com/rikimela1992/vodmax/internal/store"
	"github.com/rikimela1992/vodmax/internal/vpn"
)

// Tunnel is the part of *vpn.Tunnel the service needs.
type Tunnel interface {
	provider.Dialer
	Status() vpn.Status
	Close()
}

// StartTunnel brings a tunnel up; replaced in tests.
type StartTunnel func(ctx context.Context, cfg *vpn.Config) (Tunnel, error)

// Service is safe for concurrent use.
type Service struct {
	st    *store.Store
	box   *secret.Box
	start StartTunnel

	mu      sync.Mutex
	tunnels map[int64]Tunnel
	syncing map[int64]bool

	selMu sync.Mutex // serialises selection recomputation
	live  liveState
}

func New(st *store.Store, box *secret.Box, start StartTunnel) *Service {
	if start == nil {
		start = func(ctx context.Context, cfg *vpn.Config) (Tunnel, error) { return vpn.Start(ctx, cfg, nil) }
	}
	return &Service{st: st, box: box, start: start, tunnels: map[int64]Tunnel{}, syncing: map[int64]bool{}}
}

// Close shuts every tunnel down.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.tunnels {
		t.Close()
		delete(s.tunnels, id)
	}
}

// ErrInvalid wraps user input errors (HTTP 400).
type ErrInvalid struct{ Msg string }

func (e ErrInvalid) Error() string { return e.Msg }

func invalid(format string, args ...any) error { return ErrInvalid{fmt.Sprintf(format, args...)} }

// ---- VPN ----

// VPNView is a VPN connection as shown to the user.
type VPNView struct {
	store.VPN
	Endpoint string     `json:"endpoint"`
	Running  bool       `json:"running"`
	Status   vpn.Status `json:"status"`
}

func vpnOwner(id int64) string { return fmt.Sprintf("vpn:%d", id) }

// AddVPN validates a wg-quick configuration and stores it encrypted.
func (s *Service) AddVPN(ctx context.Context, name, conf string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return 0, invalid("nome della VPN obbligatorio (max 80 caratteri)")
	}
	if _, err := vpn.ParseConfig(strings.NewReader(conf)); err != nil {
		return 0, invalid("configurazione WireGuard non valida: %v", err)
	}
	return s.st.AddVPN(ctx, name, func(id int64) []byte { return s.box.Seal([]byte(conf), vpnOwner(id)) })
}

func (s *Service) vpnConfig(ctx context.Context, id int64) (*vpn.Config, error) {
	v, err := s.st.VPN(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, err := s.box.Open(v.Config, vpnOwner(id))
	if err != nil {
		return nil, err
	}
	return vpn.ParseConfig(bytes.NewReader(raw))
}

func (s *Service) VPNs(ctx context.Context) ([]VPNView, error) {
	list, err := s.st.VPNs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]VPNView, 0, len(list))
	for _, v := range list {
		view := VPNView{VPN: v}
		if cfg, err := s.vpnConfig(ctx, v.ID); err == nil {
			view.Endpoint = cfg.Endpoint
		}
		s.mu.Lock()
		if t := s.tunnels[v.ID]; t != nil && t.Up() {
			view.Running, view.Status = true, t.Status()
		}
		s.mu.Unlock()
		out = append(out, view)
	}
	return out, nil
}

// ConnectVPN starts (or returns) the tunnel and waits for a handshake.
func (s *Service) ConnectVPN(ctx context.Context, id int64) (vpn.Status, error) {
	t, err := s.tunnel(ctx, id)
	if err != nil {
		return vpn.Status{}, err
	}
	for {
		if st := t.Status(); st.Connected {
			return st, nil
		}
		select {
		case <-ctx.Done():
			return t.Status(), errors.New("nessun handshake con il server VPN")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// DisconnectVPN closes the tunnel; subscriptions using it stop at once.
func (s *Service) DisconnectVPN(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.tunnels[id]; t != nil {
		t.Close()
		delete(s.tunnels, id)
	}
}

func (s *Service) DeleteVPN(ctx context.Context, id int64) error {
	if err := s.st.DeleteVPN(ctx, id); err != nil {
		return err
	}
	s.DisconnectVPN(id)
	return nil
}

func (s *Service) tunnel(ctx context.Context, id int64) (Tunnel, error) {
	s.mu.Lock()
	t := s.tunnels[id]
	s.mu.Unlock()
	if t != nil && t.Up() {
		return t, nil
	}
	cfg, err := s.vpnConfig(ctx, id)
	if err != nil {
		return nil, err
	}
	t, err = s.start(ctx, cfg)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.tunnels[id]; old != nil && old.Up() {
		t.Close() // another caller won the race
		return old, nil
	}
	s.tunnels[id] = t
	return t, nil
}

// ---- Subscriptions ----

// SubscriptionInput is what the user submits. Password and URL are secrets.
type SubscriptionInput struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	VPNID    int64  `json:"vpn_id"`
	Server   string `json:"server,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	URL      string `json:"url,omitempty"`
}

type xtreamSettings struct {
	Server   string `json:"server"`
	Username string `json:"username"`
}

type subscriptionSecret struct {
	Password string `json:"password,omitempty"`
	URL      string `json:"url,omitempty"`
}

func subOwner(id int64) string { return fmt.Sprintf("subscription:%d", id) }

func (s *Service) AddSubscription(ctx context.Context, in SubscriptionInput) (int64, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		return 0, invalid("nome dell'abbonamento obbligatorio (max 80 caratteri)")
	}
	if _, err := s.st.VPN(ctx, in.VPNID); err != nil {
		return 0, invalid("ogni abbonamento richiede una connessione VPN esistente")
	}
	var settings any
	var sec subscriptionSecret
	switch in.Kind {
	case "xtream":
		if _, err := xtream.New(in.Server, in.Username, in.Password, nil); err != nil {
			return 0, invalid("%v", err)
		}
		settings = xtreamSettings{Server: strings.TrimSpace(in.Server), Username: in.Username}
		sec.Password = in.Password
	case "m3u":
		u, err := url.Parse(strings.TrimSpace(in.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return 0, invalid("URL della playlist non valido (atteso http:// o https://)")
		}
		settings = map[string]string{"host": u.Hostname()}
		sec.URL = u.String()
	default:
		return 0, invalid("tipo di abbonamento non valido: usa xtream o m3u")
	}
	rawSettings, _ := json.Marshal(settings)
	rawSecret, _ := json.Marshal(sec)
	return s.st.AddSubscription(ctx, store.Subscription{Name: in.Name, Kind: in.Kind, VPNID: in.VPNID, Settings: rawSettings},
		func(id int64) []byte { return s.box.Seal(rawSecret, subOwner(id)) })
}

// SubscriptionView adds catalog counts.
type SubscriptionView struct {
	store.Subscription
	Counts  map[string]int `json:"counts"`
	Syncing bool           `json:"syncing"`
}

func (s *Service) Subscriptions(ctx context.Context) ([]SubscriptionView, error) {
	subs, err := s.st.Subscriptions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SubscriptionView, 0, len(subs))
	for _, sub := range subs {
		counts, err := s.st.Counts(ctx, sub.ID)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		syncing := s.syncing[sub.ID]
		s.mu.Unlock()
		out = append(out, SubscriptionView{Subscription: sub, Counts: counts, Syncing: syncing})
	}
	return out, nil
}

func (s *Service) DeleteSubscription(ctx context.Context, id int64) error {
	if err := s.st.DeleteSubscription(ctx, id); err != nil {
		return err
	}
	// Variants of removed items may have been primary for other
	// subscriptions' groups.
	_, err := s.ApplySelection(ctx)
	return err
}

// ErrBusy means a synchronisation of the same subscription is running.
var ErrBusy = errors.New("sincronizzazione già in corso")

// SyncResult reports a completed synchronisation.
type SyncResult struct {
	store.ImportResult
	Skipped    int           `json:"skipped"`
	Selection  catalog.Stats `json:"selection"`
	Guide      *GuideResult  `json:"guide,omitempty"`
	GuideError string        `json:"guide_error,omitempty"`
	Duration   time.Duration `json:"duration_ms"`
}

// Sync reads the full catalog of a subscription through its VPN and
// replaces the stored one. allowDrop accepts a catalog that shrank by more
// than half.
func (s *Service) Sync(ctx context.Context, id int64, allowDrop bool) (SyncResult, error) {
	s.mu.Lock()
	if s.syncing[id] {
		s.mu.Unlock()
		return SyncResult{}, ErrBusy
	}
	s.syncing[id] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.syncing, id)
		s.mu.Unlock()
	}()

	started := time.Now()
	snap, err := s.fetch(ctx, id)
	if err != nil {
		var account json.RawMessage
		if snap != nil {
			account = snap.Account
		}
		s.st.SetSyncResult(context.Background(), id, "error", err.Error(), account)
		return SyncResult{}, err
	}
	res, err := s.st.ReplaceCatalog(ctx, id, snap.Kinds, snap.Categories, snap.Items, allowDrop)
	if err != nil {
		s.st.SetSyncResult(context.Background(), id, "error", err.Error(), snap.Account)
		return SyncResult{}, err
	}
	if err := s.st.SetSyncResult(ctx, id, "ok", "", snap.Account); err != nil {
		return SyncResult{}, err
	}
	if _, err := s.ApplySelection(ctx); err != nil {
		return SyncResult{}, err
	}
	stats, err := s.SubscriptionStats(ctx, id, nil)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{ImportResult: res, Skipped: snap.Skipped, Selection: stats}
	// The guide is refreshed with the catalog; a guide failure does not
	// invalidate the catalog.
	if sub, err := s.st.Subscription(ctx, id); err == nil && sub.Kind == "xtream" {
		if g, err := s.SyncGuide(ctx, id); err != nil {
			result.GuideError = err.Error()
		} else {
			result.Guide = &g
		}
	}
	result.Duration = time.Since(started) / time.Millisecond
	return result, nil
}

func (s *Service) fetch(ctx context.Context, id int64) (*catalog.Snapshot, error) {
	sub, err := s.st.Subscription(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, err := s.box.Open(sub.Secret, subOwner(id))
	if err != nil {
		return nil, err
	}
	var sec subscriptionSecret
	if err := json.Unmarshal(raw, &sec); err != nil {
		return nil, err
	}
	tun, err := s.tunnel(ctx, sub.VPNID)
	if err != nil {
		return nil, fmt.Errorf("VPN: %w", err)
	}
	httpClient := provider.NewHTTPClient(tun, 10*time.Minute)

	switch sub.Kind {
	case "xtream":
		var set xtreamSettings
		if err := json.Unmarshal(sub.Settings, &set); err != nil {
			return nil, err
		}
		client, err := xtream.New(set.Server, set.Username, sec.Password, httpClient)
		if err != nil {
			return nil, err
		}
		return catalog.FromXtream(ctx, client)
	case "m3u":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sec.URL, nil)
		if err != nil {
			return nil, provider.Redact(err)
		}
		req.Header.Set("User-Agent", "VodMax")
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("playlist: %w", provider.Redact(err))
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("playlist: risposta HTTP %d", resp.StatusCode)
		}
		p, err := m3u.Parse(io.LimitReader(resp.Body, 1<<30), m3u.DefaultLimits)
		if err != nil {
			return nil, err
		}
		return catalog.FromM3U(p, s.box.Seal, id), nil
	}
	return nil, fmt.Errorf("tipo di abbonamento sconosciuto %q", sub.Kind)
}

// Catalog and Categories pass through to the store.
func (s *Service) Catalog(ctx context.Context, q store.Query) (store.Page, error) {
	return s.st.Catalog(ctx, q)
}

func (s *Service) Categories(ctx context.Context, kind string, subID int64) ([]store.Category, error) {
	return s.st.Categories(ctx, kind, subID)
}
