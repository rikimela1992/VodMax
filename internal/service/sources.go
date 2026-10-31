package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/silirim/vodmax/internal/playlist/stalker"
	"github.com/silirim/vodmax/internal/playlist/xtream"
	"github.com/silirim/vodmax/internal/provider"
	"github.com/silirim/vodmax/internal/store"
)

// liveURL is the address of a live channel at the provider, for each kind
// of subscription: built (Xtream), from the playlist (M3U) or asked to the
// portal each time (Stalker).
func (s *Service) liveURL(ctx context.Context, it store.PlaybackItem) (string, error) {
	sub, err := s.st.Subscription(ctx, it.SubscriptionID)
	if err != nil {
		return "", err
	}
	switch sub.Kind {
	case "xtream":
		client, err := s.xtreamClient(ctx, it.SubscriptionID, 0)
		if err != nil {
			return "", err
		}
		return client.StreamURL(xtream.KindLive, it.SourceID, "ts"), nil
	case "m3u":
		return s.locator(it)
	case "stalker":
		cmd, err := s.locator(it)
		if err != nil {
			return "", err
		}
		raw, err := s.box.Open(sub.Secret, subOwner(sub.ID))
		if err != nil {
			return "", err
		}
		var sec subscriptionSecret
		if err := json.Unmarshal(raw, &sec); err != nil {
			return "", err
		}
		tun, err := s.tunnel(ctx, sub.VPNID)
		if err != nil {
			return "", fmt.Errorf("VPN: %w", err)
		}
		client, err := s.stalkerClient(ctx, sub, sec, provider.NewHTTPClient(tun, 30*time.Second))
		if err != nil {
			return "", err
		}
		link, err := client.Link(ctx, cmd)
		s.stalkerTokens.Store(sub.ID, client.Token())
		return link, err
	}
	return "", invalid("questo abbonamento non ha canali in diretta")
}

// locator opens the address (or portal command) kept with a title.
func (s *Service) locator(it store.PlaybackItem) (string, error) {
	if len(it.Locator) == 0 {
		return "", errors.New("indirizzo del canale mancante: sincronizza l'abbonamento")
	}
	raw, err := s.box.Open(it.Locator, fmt.Sprintf("locator:%d:%s:%s", it.SubscriptionID, it.Kind, it.SourceID))
	return string(raw), err
}

// stalkerClient builds a portal client that reuses the last session.
func (s *Service) stalkerClient(ctx context.Context, sub store.Subscription, sec subscriptionSecret, httpClient *http.Client) (*stalker.Client, error) {
	var set xtreamSettings
	if err := json.Unmarshal(sub.Settings, &set); err != nil {
		return nil, err
	}
	c, err := stalker.New(set.Server, sec.MAC, httpClient)
	if err != nil {
		return nil, err
	}
	if tok, ok := s.stalkerTokens.Load(sub.ID); ok {
		c.SetToken(tok.(string))
	}
	return c, nil
}
