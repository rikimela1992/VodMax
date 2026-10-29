package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/rikimela1992/vodmax/internal/epg"
	"github.com/rikimela1992/vodmax/internal/lineup"
	"github.com/rikimela1992/vodmax/internal/playlist/xtream"
	"github.com/rikimela1992/vodmax/internal/provider"
	"github.com/rikimela1992/vodmax/internal/store"
)

// DefaultProfile is used until profiles exist.
const DefaultProfile = 1

// GuideResult reports a guide synchronisation.
type GuideResult struct {
	Channels   int           `json:"channels"`
	Programmes int           `json:"programmes"`
	Duration   time.Duration `json:"duration_ms"`
}

// Guide window kept locally.
const (
	guidePast   = 6 * time.Hour
	guideFuture = 72 * time.Hour
)

// SyncGuide downloads the provider's XMLTV guide through the VPN and keeps
// the programmes of the channels the user can see.
func (s *Service) SyncGuide(ctx context.Context, subID int64) (GuideResult, error) {
	started := time.Now()
	client, err := s.xtreamClient(ctx, subID, 10*time.Minute)
	if err != nil {
		return GuideResult{}, err
	}
	ids, names, err := s.st.GuideChannelKeys(ctx)
	if err != nil {
		return GuideResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.XMLTVURL(), nil)
	if err != nil {
		return GuideResult{}, provider.Redact(err)
	}
	req.Header.Set("User-Agent", "VodMax")
	resp, err := client.HTTP().Do(req)
	if err != nil {
		return GuideResult{}, fmt.Errorf("guida: %w", provider.Redact(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return GuideResult{}, fmt.Errorf("guida: risposta HTTP %d", resp.StatusCode)
	}
	from, to := time.Now().Add(-guidePast), time.Now().Add(guideFuture)
	wanted := map[string]bool{}
	var channels []store.GuideChannel
	g, err := epg.Parse(io.LimitReader(resp.Body, 2<<30), epg.Options{
		OnChannel: func(c epg.Channel) {
			key := ""
			for _, n := range c.Names {
				if k := lineup.Normalize(n); names[k] {
					key = k
					break
				}
			}
			if key == "" && len(c.Names) > 0 {
				key = lineup.Normalize(c.Names[0])
			}
			channels = append(channels, store.GuideChannel{ID: c.ID, NameKey: key})
			if ids[c.ID] || names[key] {
				wanted[c.ID] = true
			}
		},
		Keep: func(ch string, start, stop time.Time) bool {
			return (wanted[ch] || ids[ch]) && stop.After(from) && start.Before(to)
		},
	})
	if err != nil {
		return GuideResult{}, err
	}
	programmes := make([]store.GuideProgramme, 0, len(g.Programmes))
	for _, p := range g.Programmes {
		programmes = append(programmes, store.GuideProgramme{Channel: p.Channel, Start: p.Start.Unix(), Stop: p.Stop.Unix(),
			Title: p.Title, Subtitle: p.Subtitle, Description: p.Description, Category: p.Category})
	}
	if err := s.st.ReplaceGuide(ctx, subID, channels, programmes); err != nil {
		return GuideResult{}, err
	}
	return GuideResult{Channels: len(wanted), Programmes: len(programmes), Duration: time.Since(started) / time.Millisecond}, nil
}

// xtreamClient builds a provider client for an Xtream subscription,
// connected through its VPN. timeout 0 means no overall timeout (streams).
func (s *Service) xtreamClient(ctx context.Context, subID int64, timeout time.Duration) (*xtream.Client, error) {
	sub, err := s.st.Subscription(ctx, subID)
	if err != nil {
		return nil, err
	}
	if sub.Kind != "xtream" {
		return nil, invalid("guida e diretta tramite server disponibili per ora solo per gli abbonamenti Xtream")
	}
	raw, err := s.box.Open(sub.Secret, subOwner(subID))
	if err != nil {
		return nil, err
	}
	var sec subscriptionSecret
	var set xtreamSettings
	if err := json.Unmarshal(raw, &sec); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(sub.Settings, &set); err != nil {
		return nil, err
	}
	tun, err := s.tunnel(ctx, sub.VPNID)
	if err != nil {
		return nil, fmt.Errorf("VPN: %w", err)
	}
	return xtream.New(set.Server, set.Username, sec.Password, provider.NewHTTPClient(tun, timeout))
}

func (s *Service) TVChannels(ctx context.Context, section string, favorites bool) ([]store.TVChannel, error) {
	switch section {
	case "", "tv", "sport", "radio":
	default:
		return nil, invalid("sezione non valida")
	}
	list, err := s.st.TVChannels(ctx, DefaultProfile, section, favorites, time.Now())
	if list == nil {
		list = []store.TVChannel{}
	}
	s.withLogos(list)
	return list, err
}

func (s *Service) ChannelGuide(ctx context.Context, itemID int64, hours int) ([]store.Programme, error) {
	if hours <= 0 || hours > 72 {
		hours = 24
	}
	now := time.Now()
	return s.st.ChannelGuide(ctx, itemID, now.Add(-2*time.Hour), now.Add(time.Duration(hours)*time.Hour))
}

func (s *Service) SetFavorite(ctx context.Context, itemID int64, favorite bool) error {
	return s.st.SetFavorite(ctx, DefaultProfile, itemID, favorite)
}

// ---- Live streams ----

// Live streams are relayed by the server: clients never reach the
// provider, which is only reachable through the internal VPN. A viewer is
// one player (browser tab, app); changing channel replaces its stream at
// once, which matters for accounts with a single connection.

type ticket struct {
	itemID  int64
	viewer  string
	expires time.Time
}

type liveStream struct {
	viewer string
	itemID int64
	cancel context.CancelFunc
	done   chan struct{}
}

type liveState struct {
	mu      sync.Mutex
	tickets map[string]ticket
	streams map[int64][]*liveStream // by subscription
}

var viewerID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// LiveTicket authorises one stream of a live channel for a viewer. The
// ticket is valid for 30 seconds and can be used once.
func (s *Service) LiveTicket(ctx context.Context, itemID int64, viewer string) (string, error) {
	if !viewerID.MatchString(viewer) {
		return "", invalid("identificativo del player non valido")
	}
	it, err := s.st.PlaybackItem(ctx, itemID)
	if err != nil {
		return "", err
	}
	if it.Kind != "live" {
		return "", invalid("non è un canale in diretta")
	}
	b := make([]byte, 24)
	rand.Read(b)
	t := hex.EncodeToString(b)
	s.live.mu.Lock()
	defer s.live.mu.Unlock()
	if s.live.tickets == nil {
		s.live.tickets, s.live.streams = map[string]ticket{}, map[int64][]*liveStream{}
	}
	now := time.Now()
	for k, v := range s.live.tickets {
		if now.After(v.expires) {
			delete(s.live.tickets, k)
		}
	}
	s.live.tickets[t] = ticket{itemID: itemID, viewer: viewer, expires: now.Add(30 * time.Second)}
	return t, nil
}

// ErrTicket means the stream ticket is unknown, used or expired.
var ErrTicket = errors.New("permesso di visione non valido o scaduto")

// ConnectionBusy is returned when every provider connection is used by
// other viewers. Force takes over the oldest one.
type ConnectionBusy struct{ Max int }

func (e ConnectionBusy) Error() string {
	return fmt.Sprintf("tutte le connessioni dell'abbonamento sono in uso (%d): chiudi un'altra visione o prendi il controllo", e.Max)
}

// LiveStream is an open relay; Close releases the provider connection.
type LiveStream struct {
	io.ReadCloser
	ContentType string
	Title       string
}

// OpenLive opens the stream authorised by ticket. The stream ends when ctx
// ends (client gone), when Close is called, or when the same viewer opens
// another channel.
func (s *Service) OpenLive(ctx context.Context, t string, force bool) (*LiveStream, error) {
	s.live.mu.Lock()
	tk, ok := s.live.tickets[t]
	delete(s.live.tickets, t)
	s.live.mu.Unlock()
	if !ok || time.Now().After(tk.expires) {
		return nil, ErrTicket
	}
	it, err := s.st.PlaybackItem(ctx, tk.itemID)
	if err != nil {
		return nil, err
	}
	sub, err := s.st.Subscription(ctx, it.SubscriptionID)
	if err != nil {
		return nil, err
	}
	var account struct {
		MaxConnections int `json:"max_connections"`
	}
	json.Unmarshal(sub.Account, &account)

	streamCtx, cancel := context.WithCancel(ctx)
	ls := &liveStream{viewer: tk.viewer, itemID: it.ID, cancel: cancel, done: make(chan struct{})}
	var stop []*liveStream
	s.live.mu.Lock()
	list := s.live.streams[it.SubscriptionID]
	kept := list[:0]
	for _, other := range list {
		if other.viewer == tk.viewer {
			stop = append(stop, other) // zapping: replace this viewer's stream
		} else {
			kept = append(kept, other)
		}
	}
	if max := account.MaxConnections; max > 0 && len(kept) >= max {
		if !force {
			s.live.mu.Unlock()
			cancel()
			return nil, ConnectionBusy{Max: max}
		}
		stop = append(stop, kept[0])
		kept = kept[1:]
	}
	s.live.streams[it.SubscriptionID] = append(kept, ls)
	s.live.mu.Unlock()

	// Wait for replaced streams to release their provider connection.
	for _, old := range stop {
		old.cancel()
		select {
		case <-old.done:
		case <-time.After(2 * time.Second):
		}
	}

	body, ctype, err := s.openUpstream(streamCtx, it)
	if err != nil {
		s.endLive(it.SubscriptionID, ls)
		return nil, err
	}
	return &LiveStream{ReadCloser: &liveBody{ReadCloser: body, end: func() { s.endLive(it.SubscriptionID, ls) }},
		ContentType: ctype, Title: it.Title}, nil
}

func (s *Service) endLive(subID int64, ls *liveStream) {
	ls.cancel()
	s.live.mu.Lock()
	list := s.live.streams[subID]
	for i, other := range list {
		if other == ls {
			s.live.streams[subID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	s.live.mu.Unlock()
	select {
	case <-ls.done:
	default:
		close(ls.done)
	}
}

type liveBody struct {
	io.ReadCloser
	once sync.Once
	end  func()
}

func (b *liveBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.end)
	return err
}

// openUpstream requests the channel from the provider through the VPN. A
// provider may take a moment to release the previous connection of a
// single-connection account, so refusals are retried briefly.
func (s *Service) openUpstream(ctx context.Context, it store.PlaybackItem) (io.ReadCloser, string, error) {
	client, err := s.xtreamClient(ctx, it.SubscriptionID, 0)
	if err != nil {
		return nil, "", err
	}
	url := client.StreamURL(xtream.KindLive, it.SourceID, "ts")
	var lastStatus int
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, "", ctx.Err()
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("User-Agent", "VodMax")
		resp, err := client.HTTP().Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("canale: %w", provider.Redact(err))
		}
		if resp.StatusCode == http.StatusOK {
			ctype := resp.Header.Get("Content-Type")
			if ctype == "" || ctype == "application/octet-stream" {
				ctype = "video/mp2t"
			}
			return resp.Body, ctype, nil
		}
		lastStatus = resp.StatusCode
		resp.Body.Close()
	}
	return nil, "", fmt.Errorf("il provider ha rifiutato il canale (HTTP %d)", lastStatus)
}
