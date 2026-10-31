package service

import (
	"context"
	"crypto/subtle"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/silirim/vodmax/internal/store"
)

// Output to other apps: VodMax shows its channels as an HDHomeRun tuner
// (Plex, Jellyfin, Emby find it and record with their own DVR) and as an
// M3U playlist with its XMLTV guide (TiviMate, Kodi, VLC...). Streams go
// through VodMax like its own: one provider connection per channel, the
// limits and the VPN of the subscription. Off by default; the addresses
// carry a secret key, because players and tuners cannot log in.

const outputKey = "output"

// OutputSettings: Key is in every address; changing it cuts off whoever
// had the old addresses.
type OutputSettings struct {
	Enabled bool   `json:"enabled"`
	Key     string `json:"key"`
}

// ErrOutputOff means the output is off or the key is wrong.
var ErrOutputOff = errors.New("uscita verso altre app spenta o indirizzo non valido")

func (s *Service) OutputSettings(ctx context.Context) (OutputSettings, error) {
	var o OutputSettings
	err := s.st.Setting(ctx, outputKey, &o)
	return o, err
}

// SetOutput turns the output on or off; newKey replaces the key.
func (s *Service) SetOutput(ctx context.Context, enabled, newKey bool) (OutputSettings, error) {
	o, err := s.OutputSettings(ctx)
	if err != nil {
		return o, err
	}
	o.Enabled = enabled
	if o.Key == "" || newKey {
		o.Key = randomToken()[:32]
	}
	return o, s.st.SetSetting(ctx, outputKey, o)
}

func (s *Service) checkOutput(ctx context.Context, key string) error {
	o, err := s.OutputSettings(ctx)
	if err != nil {
		return err
	}
	if !o.Enabled || o.Key == "" || subtle.ConstantTimeCompare([]byte(key), []byte(o.Key)) != 1 {
		return ErrOutputOff
	}
	return nil
}

// OutputChannel is a channel as the other apps see it.
type OutputChannel struct {
	ID      int64
	Number  int
	Name    string
	Section string
	Logo    string // signed local path
	GuideID string
}

// OutputChannels lists the visible channels in remote-control order, as
// for the main profile (no adult channels).
func (s *Service) OutputChannels(ctx context.Context, key string) ([]OutputChannel, error) {
	if err := s.checkOutput(ctx, key); err != nil {
		return nil, err
	}
	list, err := s.st.TVChannels(ctx, DefaultProfile, true, "", false, time.Now())
	if err != nil {
		return nil, err
	}
	s.withLogos(list)
	out := make([]OutputChannel, 0, len(list))
	for _, c := range list {
		out = append(out, OutputChannel{ID: c.ID, Number: c.Number, Name: c.Name, Section: c.Section, Logo: c.Logo, GuideID: c.GuideID})
	}
	return out, nil
}

// OutputTuners is how many channels can play at once: the connections of
// the subscriptions (at least 1).
func (s *Service) OutputTuners(ctx context.Context) int {
	subs, err := s.st.Subscriptions(ctx)
	if err != nil {
		return 1
	}
	n := 0
	for _, sub := range subs {
		if sub.Kind == "local" {
			continue
		}
		if m, err := s.maxConnections(ctx, sub.ID); err == nil {
			n += m
		}
	}
	return min(max(n, 1), 8)
}

// OpenOutput opens a channel for another app, like OpenLive.
func (s *Service) OpenOutput(ctx context.Context, key string, channelID int64) (*Stream, error) {
	if err := s.checkOutput(ctx, key); err != nil {
		return nil, err
	}
	it, err := s.st.PlaybackItem(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if it.Kind != "live" {
		return nil, store.ErrNotFound
	}
	t := randomToken()
	s.live.mu.Lock()
	if s.live.tickets == nil {
		s.live.tickets = map[string]ticket{}
	}
	// Each request is its own viewer: two channels at once take two
	// connections, and a channel left by the app frees its own.
	s.live.tickets[t] = ticket{itemID: channelID, viewer: "app-" + t[:16], expires: time.Now().Add(30 * time.Second)}
	s.live.mu.Unlock()
	return s.OpenLive(ctx, t, false)
}

// WriteOutputGuide writes the XMLTV guide of the output channels, from a
// day ago to the end of the guide; base is the absolute address of the
// server (for the logos).
func (s *Service) WriteOutputGuide(ctx context.Context, w io.Writer, key, base string) error {
	channels, err := s.OutputChannels(ctx, key)
	if err != nil {
		return err
	}
	var guides []string
	for _, c := range channels {
		if c.GuideID != "" {
			guides = append(guides, c.GuideID)
		}
	}
	now := time.Now()
	progs, err := s.st.GuideWindow(ctx, guides, now.Add(-24*time.Hour), now.Add(guideFuture))
	if err != nil {
		return err
	}
	type text struct {
		Lang  string `xml:"lang,attr,omitempty"`
		Value string `xml:",chardata"`
	}
	type icon struct {
		Src string `xml:"src,attr"`
	}
	type channel struct {
		XMLName xml.Name `xml:"channel"`
		ID      string   `xml:"id,attr"`
		Names   []text   `xml:"display-name"`
		Icon    *icon    `xml:"icon"`
	}
	type programme struct {
		XMLName  xml.Name  `xml:"programme"`
		Start    string    `xml:"start,attr"`
		Stop     string    `xml:"stop,attr"`
		Channel  string    `xml:"channel,attr"`
		Title    text      `xml:"title"`
		Sub      *text     `xml:"sub-title"`
		Desc     *text     `xml:"desc"`
		Category *text     `xml:"category"`
		Live     *struct{} `xml:"live"`
		Previous *struct{} `xml:"previously-shown"`
		Premiere *struct{} `xml:"premiere"`
		New      *struct{} `xml:"new"`
	}
	opt := func(s string) *text {
		if s == "" {
			return nil
		}
		return &text{Value: s}
	}
	stamp := func(t int64) string { return time.Unix(t, 0).UTC().Format("20060102150405 +0000") }
	io.WriteString(w, xml.Header+`<tv generator-info-name="VodMax">`+"\n")
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	for _, c := range channels {
		ch := channel{ID: outputChannelID(c), Names: []text{{Value: c.Name}, {Value: fmt.Sprint(c.Number)}}}
		if c.Logo != "" {
			ch.Icon = &icon{Src: base + c.Logo}
		}
		if err := enc.Encode(ch); err != nil {
			return err
		}
	}
	for _, c := range channels {
		for _, p := range progs[c.GuideID] {
			x := programme{Start: stamp(p.Start), Stop: stamp(p.Stop), Channel: outputChannelID(c), Title: text{Value: p.Title},
				Sub: opt(p.Subtitle), Desc: opt(p.Description), Category: opt(p.Category)}
			switch p.Airing {
			case "live":
				x.Live = &struct{}{}
			case "rerun":
				x.Previous = &struct{}{}
			case "premiere":
				x.Premiere = &struct{}{}
			case "new":
				x.New = &struct{}{}
			}
			if err := enc.Encode(x); err != nil {
				return err
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	enc.Flush()
	_, err = io.WriteString(w, "\n</tv>\n")
	return err
}

func outputChannelID(c OutputChannel) string { return fmt.Sprintf("vodmax.%d", c.ID) }

// WriteOutputPlaylist writes the M3U playlist of the output channels.
func (s *Service) WriteOutputPlaylist(ctx context.Context, w io.Writer, key, base string) error {
	channels, err := s.OutputChannels(ctx, key)
	if err != nil {
		return err
	}
	clean := func(s string) string { return strings.NewReplacer(`"`, "'", "\n", " ", "\r", " ", ",", " ").Replace(s) }
	fmt.Fprintf(w, "#EXTM3U url-tvg=\"%s/out/%s/epg.xml\"\n", base, key)
	group := map[string]string{"tv": "TV", "sport": "Sport", "radio": "Radio"}
	for _, c := range channels {
		logo := ""
		if c.Logo != "" {
			logo = base + c.Logo
		}
		fmt.Fprintf(w, "#EXTINF:-1 tvg-id=\"%s\" tvg-chno=\"%d\" tvg-name=\"%s\" tvg-logo=\"%s\" group-title=\"%s\",%s\n%s/out/%s/channel/%d.ts\n",
			outputChannelID(c), c.Number, clean(c.Name), logo, group[c.Section], strings.ReplaceAll(c.Name, "\n", " "), base, key, c.ID)
	}
	return nil
}

// OutputLineup is the HDHomeRun channel list.
type OutputLineup struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	URL         string `json:"URL"`
	HD          int    `json:"HD,omitempty"`
}

func (s *Service) OutputLineup(ctx context.Context, key, base string) ([]OutputLineup, error) {
	channels, err := s.OutputChannels(ctx, key)
	if err != nil {
		return nil, err
	}
	out := make([]OutputLineup, 0, len(channels))
	for _, c := range channels {
		out = append(out, OutputLineup{GuideNumber: fmt.Sprint(c.Number), GuideName: c.Name, HD: 1,
			URL: fmt.Sprintf("%s/out/%s/channel/%d.ts", base, key, c.ID)})
	}
	return out, nil
}

// OutputDevice is the HDHomeRun discover.json; DeviceID comes from the key,
// so it changes with it.
func (s *Service) OutputDevice(ctx context.Context, key, base string) (map[string]any, error) {
	if err := s.checkOutput(ctx, key); err != nil {
		return nil, err
	}
	id := strings.ToUpper(key[:8])
	url := base + "/out/" + key
	return map[string]any{
		"FriendlyName": "VodMax", "Manufacturer": "VodMax", "ModelNumber": "HDTC-2US", "FirmwareName": "hdhomeruntc_atsc",
		"FirmwareVersion": "20200101", "DeviceID": id, "DeviceAuth": "vodmax", "BaseURL": url, "LineupURL": url + "/lineup.json",
		"TunerCount": s.OutputTuners(ctx),
	}, nil
}

// OutputLinks are the addresses shown in the settings.
func OutputLinks(base string, o OutputSettings) map[string]string {
	u := base + "/out/" + o.Key
	return map[string]string{"tuner": u, "playlist": u + "/playlist.m3u", "guide": u + "/epg.xml"}
}
