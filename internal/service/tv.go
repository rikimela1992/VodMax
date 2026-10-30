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

	"github.com/silirim/vodmax/internal/epg"
	"github.com/silirim/vodmax/internal/lineup"
	"github.com/silirim/vodmax/internal/playlist/xtream"
	"github.com/silirim/vodmax/internal/provider"
	"github.com/silirim/vodmax/internal/store"
)

// DefaultProfile is created with the database (the first admin profile)
// and used by internal work that has no viewer.
const DefaultProfile = 1

// GuideResult reports a guide synchronisation.
type GuideResult struct {
	Channels   int           `json:"channels"`
	Programmes int           `json:"programmes"`
	Duration   time.Duration `json:"duration_ms"`
}

// Guide window kept locally.
const (
	guidePast   = store.GuideHistory // for catch-up
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
	if err := s.st.ReplaceGuide(ctx, fmt.Sprintf("sub:%d", subID), channels, programmes); err != nil {
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
	v := viewerOf(ctx)
	list, err := s.st.TVChannels(ctx, v.ProfileID, v.Kids, section, favorites, time.Now())
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
	if err := s.allowItem(ctx, itemID); err != nil {
		return nil, err
	}
	now := time.Now()
	return s.st.ChannelGuide(ctx, itemID, now.Add(-2*time.Hour), now.Add(time.Duration(hours)*time.Hour))
}

func (s *Service) SetFavorite(ctx context.Context, itemID int64, favorite bool) error {
	if err := s.allowItem(ctx, itemID); err != nil {
		return err
	}
	return s.st.SetFavorite(ctx, viewerOf(ctx).ProfileID, itemID, favorite)
}

// ---- Live streams ----

// Live streams are relayed by the server: clients never reach the
// provider, which is only reachable through the internal VPN. Opening a
// stream needs a single-use ticket, because video players cannot send an
// Authorization header.

type ticket struct {
	itemID  int64
	viewer  string
	expires time.Time
}

type liveState struct {
	mu      sync.Mutex
	tickets map[string]ticket
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
	if err := s.allowItem(ctx, itemID); err != nil {
		return "", err
	}
	t := randomToken()
	s.live.mu.Lock()
	defer s.live.mu.Unlock()
	if s.live.tickets == nil {
		s.live.tickets = map[string]ticket{}
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

func randomToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ErrTicket means the stream ticket is unknown, used or expired.
var ErrTicket = errors.New("permesso di visione non valido o scaduto")

// OpenLive opens the stream authorised by ticket. It ends when ctx ends
// (client gone), when Close is called, or when the same viewer opens
// another stream.
func (s *Service) OpenLive(ctx context.Context, t string, force bool) (*Stream, error) {
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
	// A channel being recorded is watched from the recording: no second
	// provider connection is needed.
	if rec := s.activeRecordingFor(ctx, it.ID); rec != nil {
		return s.followRecording(ctx, rec)
	}
	streamCtx, release, err := s.acquire(ctx, it.SubscriptionID, tk.viewer, force, false)
	if err != nil {
		return nil, err
	}
	client, err := s.xtreamClient(ctx, it.SubscriptionID, 0)
	if err != nil {
		release()
		return nil, err
	}
	resp, err := s.upstream(streamCtx, it.SubscriptionID, client.StreamURL(xtream.KindLive, it.SourceID, "ts"), "")
	if err != nil {
		release()
		return nil, err
	}
	ctype := resp.Header.Get("Content-Type")
	if ctype == "" || ctype == "application/octet-stream" {
		ctype = "video/mp2t"
	}
	return &Stream{ReadCloser: &releasingBody{ReadCloser: resp.Body, release: release}, ContentType: ctype, Title: it.Title}, nil
}
