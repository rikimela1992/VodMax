package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rikimela1992/vodmax/internal/playlist/xtream"
	"github.com/rikimela1992/vodmax/internal/store"
)

// ---- Series episodes ----

const episodesFreshFor = 12 * time.Hour

// Episodes returns a series' episodes, loading them from the provider
// (through the VPN) when missing or older than episodesFreshFor.
func (s *Service) Episodes(ctx context.Context, seriesItemID int64) ([]store.Episode, error) {
	it, err := s.st.PlaybackItem(ctx, seriesItemID)
	if err != nil {
		return nil, err
	}
	if it.Kind != "series" {
		return nil, invalid("non è una serie")
	}
	if err := s.allowItem(ctx, seriesItemID); err != nil {
		return nil, err
	}
	profile := viewerOf(ctx).ProfileID
	fetched, err := s.st.SeriesFetchedAt(ctx, seriesItemID)
	if err != nil {
		return nil, err
	}
	if time.Since(fetched) > episodesFreshFor {
		if err := s.loadEpisodes(ctx, it); err != nil {
			// Stale episodes are better than none.
			if eps, serr := s.st.Episodes(ctx, profile, seriesItemID); serr == nil && len(eps) > 0 {
				return eps, nil
			}
			return nil, err
		}
	}
	return s.st.Episodes(ctx, profile, seriesItemID)
}

func (s *Service) loadEpisodes(ctx context.Context, it store.PlaybackItem) error {
	client, err := s.xtreamClient(ctx, it.SubscriptionID, time.Minute)
	if err != nil {
		return err
	}
	info, err := client.SeriesInfo(ctx, it.SourceID)
	if err != nil {
		return err
	}
	eps := make([]store.Episode, 0, len(info.Episodes))
	for _, e := range info.Episodes {
		if e.ID == "" {
			continue
		}
		title := episodeTitle(e.Title.String())
		eps = append(eps, store.Episode{SourceID: e.ID.String(), Season: int(e.Season.Int()), Number: int(e.EpisodeNum.Int()),
			Title: title, Extension: e.ContainerExtension.String(), Duration: int(e.Info.DurationSecs.Int()),
			Plot: strings.TrimSpace(e.Info.Plot.String())})
	}
	return s.st.ReplaceEpisodes(ctx, it.ID, eps)
}

// Providers often name episodes "Series (year) - S01E01 - Title": keep
// only the title.
var episodeCode = regexp.MustCompile(`(?i)\bS\d{1,3}\s*E\d{1,4}\b\s*[-–:]?\s*`)

func episodeTitle(raw string) string {
	raw = strings.TrimSpace(raw)
	if loc := episodeCode.FindStringIndex(raw); loc != nil {
		if rest := strings.TrimSpace(raw[loc[1]:]); rest != "" {
			return rest
		}
	}
	return raw
}

// ---- Progress ----

func (s *Service) SaveProgress(ctx context.Context, media string, id int64, position, duration int) (store.Progress, error) {
	switch media {
	case "movie":
		it, err := s.st.PlaybackItem(ctx, id)
		if err != nil {
			return store.Progress{}, err
		}
		if it.Kind != "movie" {
			return store.Progress{}, invalid("non è un film")
		}
		if err := s.allowItem(ctx, id); err != nil {
			return store.Progress{}, err
		}
	case "episode":
		_, e, err := s.st.EpisodePlayback(ctx, id)
		if err != nil {
			return store.Progress{}, err
		}
		if err := s.allowItem(ctx, e.SeriesItemID); err != nil {
			return store.Progress{}, err
		}
	default:
		return store.Progress{}, invalid("tipo non valido: movie o episode")
	}
	p, err := s.st.SaveProgress(ctx, viewerOf(ctx).ProfileID, media, id, position, duration)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return p, invalid("%v", err)
	}
	return p, err
}

// Progress returns the saved position of a film or episode (nil if none).
func (s *Service) Progress(ctx context.Context, media string, id int64) (*store.Progress, error) {
	if media != "movie" && media != "episode" {
		return nil, invalid("tipo non valido: movie o episode")
	}
	return s.st.GetProgress(ctx, viewerOf(ctx).ProfileID, media, id)
}

func (s *Service) Continue(ctx context.Context) ([]store.ContinueEntry, error) {
	v := viewerOf(ctx)
	list, err := s.st.Continue(ctx, v.ProfileID, v.Kids, 30)
	for i := range list {
		if list[i].ImageAllowed {
			list[i].Logo = s.LogoURL(list[i].ImageItemID)
		}
	}
	return list, err
}

// ---- Playback sessions ----

// A playback session covers one film or episode being watched by one
// viewer. "direct" sessions relay the provider's file as is, with Range
// requests (MP4 with H.264: browsers play it natively and can seek). Other
// files are remuxed by ffmpeg into fragmented MP4 from a given position
// ("remux"); ffmpeg reads them through a loopback-only relay, so it never
// reaches the provider except through the VPN.

type vodSession struct {
	id       string
	viewer   string
	media    string
	mediaID  int64
	subID    int64
	url      string // provider URL with credentials: never leaves the server
	title    string
	mode     string
	probe    *probeResult
	expires  time.Time
	internal string // loopback token for ffmpeg
	subDir   string // temporary folder of extracted subtitles
	subFile  string // WebVTT being written by the current remux
}

// drop removes what a session left on disk.
func (v *vodSession) drop() {
	if v.subDir != "" {
		os.RemoveAll(v.subDir)
	}
}

type vodState struct {
	mu       sync.Mutex
	sessions map[string]*vodSession
	byToken  map[string]*vodSession
	loopback string // "127.0.0.1:port" of the ffmpeg relay
	once     sync.Once
	ffmpeg   string
	ffprobe  string
}

// SetFFmpeg sets the ffmpeg and ffprobe executables (default: from PATH).
func (s *Service) SetFFmpeg(ffmpeg, ffprobe string) {
	s.vod.ffmpeg, s.vod.ffprobe = ffmpeg, ffprobe
}

// PlaybackSession is what a client needs to play.
type PlaybackSession struct {
	ID       string          `json:"id"`
	Mode     string          `json:"mode"` // direct or remux
	URL      string          `json:"url"`
	Title    string          `json:"title"`
	Duration int             `json:"duration,omitempty"` // seconds, when known
	Resume   *store.Progress `json:"resume,omitempty"`
	// Tracks found in the file (empty if it could not be analysed), and
	// the suggested choice: Italian audio when present, no subtitles.
	Audio        []Track `json:"audio"`
	Subtitles    []Track `json:"subtitles"`
	DefaultAudio int     `json:"default_audio"`
	// SubtitlesURL returns the WebVTT of the chosen subtitles, written
	// while the remux runs (?sub=N on the remux URL).
	SubtitlesURL string `json:"subtitles_url,omitempty"`
}

// Track is an audio or subtitle stream; Index counts within its type.
type Track struct {
	Index     int    `json:"index"`
	Language  string `json:"language,omitempty"`
	Title     string `json:"title,omitempty"`
	Codec     string `json:"codec"`
	Channels  int    `json:"channels,omitempty"`
	Default   bool   `json:"default,omitempty"`
	Forced    bool   `json:"forced,omitempty"`
	Supported bool   `json:"supported"` // subtitles: text, so the browser can show them
}

// NewPlaybackSession prepares a film or episode for a viewer. mode may
// force "remux" (e.g. after the browser failed to play the direct file).
func (s *Service) NewPlaybackSession(ctx context.Context, media string, id int64, viewer, mode string) (PlaybackSession, error) {
	if !viewerID.MatchString(viewer) {
		return PlaybackSession{}, invalid("identificativo del player non valido")
	}
	var it store.PlaybackItem
	var duration int
	title := ""
	kind := xtream.KindMovie
	switch media {
	case "movie":
		var err error
		if it, err = s.st.PlaybackItem(ctx, id); err != nil {
			return PlaybackSession{}, err
		}
		if it.Kind != "movie" {
			return PlaybackSession{}, invalid("non è un film")
		}
		if err := s.allowItem(ctx, id); err != nil {
			return PlaybackSession{}, err
		}
		title = it.Title
	case "episode":
		var e store.Episode
		var err error
		if it, e, err = s.st.EpisodePlayback(ctx, id); err != nil {
			return PlaybackSession{}, err
		}
		if err := s.allowItem(ctx, e.SeriesItemID); err != nil {
			return PlaybackSession{}, err
		}
		kind, duration = xtream.KindEpisode, e.Duration
		title = fmt.Sprintf("%s · S%dE%d %s", it.Title, e.Season, e.Number, e.Title)
	default:
		return PlaybackSession{}, invalid("tipo non valido: movie o episode")
	}
	client, err := s.xtreamClient(ctx, it.SubscriptionID, time.Minute)
	if err != nil {
		return PlaybackSession{}, err
	}
	ext := strings.ToLower(it.Extension)
	auto := mode == ""
	if mode == "" {
		mode = "remux"
		if ext == "mp4" || ext == "m4v" {
			mode = "direct"
		}
	}
	if mode != "direct" && mode != "remux" {
		return PlaybackSession{}, invalid("modalità non valida")
	}
	sess := &vodSession{id: randomToken(), viewer: viewer, media: media, mediaID: id, subID: it.SubscriptionID,
		url: client.StreamURL(kind, it.SourceID, ext), title: title, mode: mode, internal: randomToken(),
		expires: time.Now().Add(8 * time.Hour)}
	s.vod.mu.Lock()
	if s.vod.sessions == nil {
		s.vod.sessions, s.vod.byToken = map[string]*vodSession{}, map[string]*vodSession{}
	}
	for k, old := range s.vod.sessions {
		if old.viewer == viewer || time.Now().After(old.expires) {
			delete(s.vod.sessions, k)
			delete(s.vod.byToken, old.internal)
			old.drop()
		}
	}
	s.vod.sessions[sess.id] = sess
	s.vod.byToken[sess.internal] = sess
	s.vod.mu.Unlock()

	out := PlaybackSession{ID: sess.id, Mode: mode, Title: title, Duration: duration, URL: "/stream/vod/" + sess.id,
		Audio: []Track{}, Subtitles: []Track{}}
	// The analysis lists the tracks; a direct file can still play without
	// it, so it does not wait long (an MP4 with its index at the end needs
	// several requests).
	probeCtx, cancel := ctx, context.CancelFunc(func() {})
	if mode == "direct" {
		probeCtx, cancel = context.WithTimeout(ctx, directProbeLimit)
	}
	probe, err := s.probe(probeCtx, sess)
	cancel()
	if err != nil && mode == "remux" {
		return PlaybackSession{}, err
	}
	if err == nil {
		sess.probe = probe
		if probe.Duration > 0 {
			out.Duration = int(probe.Duration)
		}
		out.Audio, out.Subtitles, out.DefaultAudio = probe.Audio, probe.Subtitles, probe.preferredAudio()
		out.SubtitlesURL = "/stream/vod/" + sess.id + "/subtitles"
		// A browser plays the first audio track of a direct file: when
		// the preferred one is another, convert with it.
		if mode == "direct" && auto && out.DefaultAudio != 0 {
			mode, sess.mode, out.Mode = "remux", "remux", "remux"
		}
	}
	if mode == "remux" {
		out.URL += "/remux"
	}
	if p, err := s.st.GetProgress(ctx, viewerOf(ctx).ProfileID, media, id); err == nil && p != nil && !p.Completed && p.Position > 30 {
		out.Resume = p
	}
	return out, nil
}

// ErrSession means the playback session is unknown or expired.
var ErrSession = errors.New("sessione di riproduzione non valida o scaduta")

func (s *Service) session(id string) (*vodSession, error) {
	s.vod.mu.Lock()
	defer s.vod.mu.Unlock()
	sess := s.vod.sessions[id]
	if sess == nil || time.Now().After(sess.expires) {
		return nil, ErrSession
	}
	return sess, nil
}

// OpenDirect relays (a range of) the provider file. Each request replaces
// the viewer's previous stream: browsers abandon the old request when
// seeking.
func (s *Service) OpenDirect(ctx context.Context, sessionID, rangeHeader string, force bool) (*Stream, error) {
	sess, err := s.session(sessionID)
	if err != nil {
		return nil, err
	}
	if sess.mode != "direct" {
		return nil, invalid("sessione non diretta")
	}
	streamCtx, release, err := s.acquire(ctx, sess.subID, sess.viewer, force, false)
	if err != nil {
		return nil, err
	}
	resp, err := s.upstream(streamCtx, sess.subID, sess.url, rangeHeader)
	if err != nil {
		release()
		return nil, err
	}
	ctype := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ctype, "video/") {
		ctype = "video/mp4"
	}
	return &Stream{ReadCloser: &releasingBody{ReadCloser: resp.Body, release: release}, ContentType: ctype, Title: sess.title,
		Status: resp.StatusCode, ContentLength: resp.ContentLength, ContentRange: resp.Header.Get("Content-Range")}, nil
}

const directProbeLimit = 4 * time.Second

type probeResult struct {
	Duration   float64
	VideoCodec string
	Audio      []Track
	Subtitles  []Track
}

// textSubtitles can be converted to WebVTT; image subtitles (PGS, DVD,
// DVB) cannot be shown by browsers.
var textSubtitles = map[string]bool{"subrip": true, "ass": true, "ssa": true, "mov_text": true, "webvtt": true, "text": true}

// parseProbe reads ffprobe's JSON.
func parseProbe(out []byte) (*probeResult, error) {
	var raw struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type        string            `json:"codec_type"`
			Codec       string            `json:"codec_name"`
			Channels    int               `json:"channels"`
			Tags        map[string]string `json:"tags"`
			Disposition map[string]int    `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	p := &probeResult{Audio: []Track{}, Subtitles: []Track{}}
	p.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	for _, st := range raw.Streams {
		t := Track{Codec: st.Codec, Channels: st.Channels, Language: strings.ToLower(st.Tags["language"]), Title: st.Tags["title"],
			Default: st.Disposition["default"] == 1, Forced: st.Disposition["forced"] == 1}
		if t.Language == "und" {
			t.Language = ""
		}
		switch st.Type {
		case "video":
			if p.VideoCodec == "" && st.Disposition["attached_pic"] == 0 {
				p.VideoCodec = st.Codec
			}
		case "audio":
			t.Index, t.Supported = len(p.Audio), true
			p.Audio = append(p.Audio, t)
		case "subtitle":
			t.Index, t.Supported = len(p.Subtitles), textSubtitles[st.Codec]
			p.Subtitles = append(p.Subtitles, t)
		}
	}
	return p, nil
}

// preferredAudio: Italian, else the file's default, else the first.
func (p *probeResult) preferredAudio() int {
	for _, t := range p.Audio {
		if t.Language == "ita" || t.Language == "it" {
			return t.Index
		}
	}
	for _, t := range p.Audio {
		if t.Default {
			return t.Index
		}
	}
	return 0
}

// probe reads duration and codecs with ffprobe through the loopback relay.
func (s *Service) probe(ctx context.Context, sess *vodSession) (*probeResult, error) {
	addr, err := s.loopback()
	if err != nil {
		return nil, err
	}
	streamCtx, release, err := s.acquire(ctx, sess.subID, sess.viewer, false, false)
	if err != nil {
		return nil, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(streamCtx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.ffprobeBin(), "-v", "error", "-print_format", "json", "-show_format", "-show_streams",
		"http://"+addr+"/internal/"+sess.internal)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("analisi del file non riuscita: %s", firstLine(stderr.String(), err))
	}
	return parseProbe(out)
}

func firstLine(s string, err error) string {
	if line, _, _ := strings.Cut(strings.TrimSpace(s), "\n"); line != "" {
		return line
	}
	return err.Error()
}

// OpenRemux starts ffmpeg at position (seconds) and returns fragmented
// MP4. H.264 video and AAC audio are copied; other audio is converted to
// AAC stereo, other video to H.264 (slow on small machines).
// audio and sub choose tracks by index (-1: first audio, no subtitles).
func (s *Service) OpenRemux(ctx context.Context, sessionID string, position float64, force bool, audio, sub int) (*Stream, error) {
	sess, err := s.session(sessionID)
	if err != nil {
		return nil, err
	}
	if sess.mode != "remux" || sess.probe == nil {
		return nil, invalid("sessione non convertibile")
	}
	addr, err := s.loopback()
	if err != nil {
		return nil, err
	}
	streamCtx, release, err := s.acquire(ctx, sess.subID, sess.viewer, force, false)
	if err != nil {
		return nil, err
	}
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error"}
	if position > 0 {
		args = append(args, "-ss", strconv.FormatFloat(position, 'f', 1, 64))
	}
	p := sess.probe
	audioCodec := ""
	if audio < 0 || audio >= len(p.Audio) {
		audio = 0
	}
	if len(p.Audio) > 0 {
		audioCodec = p.Audio[audio].Codec
	}
	args = append(args, "-i", "http://"+addr+"/internal/"+sess.internal,
		"-map", "0:v:0", "-map", "0:a:"+strconv.Itoa(audio)+"?", "-sn", "-dn")
	if sess.probe.VideoCodec == "h264" {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p")
	}
	if audioCodec == "aac" {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "192k")
	}
	args = append(args, "-f", "mp4", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "pipe:1")
	// Text subtitles go to a second output, read by the player as it grows.
	subFile := ""
	if sub >= 0 && sub < len(p.Subtitles) && p.Subtitles[sub].Supported {
		var err error
		if subFile, err = s.subtitleFile(sess, sub, position); err != nil {
			release()
			return nil, err
		}
		args = append(args, "-map", "0:s:"+strconv.Itoa(sub), "-c:s", "webvtt", "-flush_packets", "1", "-f", "webvtt", subFile)
	}
	s.vod.mu.Lock()
	sess.subFile = subFile
	s.vod.mu.Unlock()
	cmd := exec.CommandContext(streamCtx, s.ffmpegBin(), args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		release()
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, n: 4096}
	if err := cmd.Start(); err != nil {
		release()
		return nil, fmt.Errorf("ffmpeg non avviabile: %w", err)
	}
	body := &cmdBody{ReadCloser: stdout, cmd: cmd}
	// Wait for the first bytes so errors are reported, not an empty video.
	br := bufio.NewReaderSize(body, 64<<10)
	if _, err := br.Peek(8); err != nil {
		body.Close()
		release()
		return nil, fmt.Errorf("conversione non riuscita: %s", firstLine(stderr.String(), err))
	}
	return &Stream{ReadCloser: &releasingBody{ReadCloser: readCloser{br, body}, release: release}, ContentType: "video/mp4",
		Title: sess.title}, nil
}

type cmdBody struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (b *cmdBody) Close() error {
	b.ReadCloser.Close()
	if b.cmd.Process != nil {
		b.cmd.Process.Kill()
	}
	b.cmd.Wait()
	return nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}

func (s *Service) ffmpegBin() string {
	if s.vod.ffmpeg != "" {
		return s.vod.ffmpeg
	}
	return "ffmpeg"
}

func (s *Service) ffprobeBin() string {
	if s.vod.ffprobe != "" {
		return s.vod.ffprobe
	}
	return "ffprobe"
}

// loopback starts (once) the relay used by ffmpeg and ffprobe. It listens
// on 127.0.0.1 only and serves only session tokens it issued; requests are
// forwarded to the provider through the VPN with their Range header. The
// provider connection is the one already reserved by the session.
func (s *Service) loopback() (string, error) {
	var err error
	s.vod.once.Do(func() {
		var l net.Listener
		l, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return
		}
		s.vod.loopback = l.Addr().String()
		srv := &http.Server{Handler: http.HandlerFunc(s.serveLoopback), ReadHeaderTimeout: 10 * time.Second}
		go srv.Serve(l)
	})
	if s.vod.loopback == "" {
		if err == nil {
			err = errors.New("relay locale non disponibile")
		}
		return "", err
	}
	return s.vod.loopback, nil
}

func (s *Service) serveLoopback(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/internal/")
	s.vod.mu.Lock()
	sess := s.vod.byToken[token]
	s.vod.mu.Unlock()
	if sess == nil || r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	resp, err := s.upstream(r.Context(), sess.subID, sess.url, r.Header.Get("Range"))
	if err != nil {
		http.Error(w, "upstream", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (s *Service) subtitleFile(sess *vodSession, sub int, position float64) (string, error) {
	s.vod.mu.Lock()
	defer s.vod.mu.Unlock()
	if sess.subDir == "" {
		dir, err := os.MkdirTemp("", "vodmax-subs-")
		if err != nil {
			return "", err
		}
		sess.subDir = dir
	} else {
		// One file at a time per session.
		old, _ := filepath.Glob(filepath.Join(sess.subDir, "*.vtt"))
		for _, f := range old {
			os.Remove(f)
		}
	}
	return filepath.Join(sess.subDir, fmt.Sprintf("%d-%d.vtt", sub, int(position))), nil
}

// Subtitles returns the WebVTT written so far by the session's remux (cue
// times start from the remux position).
func (s *Service) Subtitles(sessionID string) ([]byte, error) {
	sess, err := s.session(sessionID)
	if err != nil {
		return nil, err
	}
	s.vod.mu.Lock()
	file := sess.subFile
	s.vod.mu.Unlock()
	if file == "" {
		return []byte("WEBVTT\n"), nil
	}
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return []byte("WEBVTT\n"), nil
	}
	return data, err
}
