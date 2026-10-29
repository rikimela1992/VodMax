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
	"os/exec"
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
	fetched, err := s.st.SeriesFetchedAt(ctx, seriesItemID)
	if err != nil {
		return nil, err
	}
	if time.Since(fetched) > episodesFreshFor {
		if err := s.loadEpisodes(ctx, it); err != nil {
			// Stale episodes are better than none.
			if eps, serr := s.st.Episodes(ctx, DefaultProfile, seriesItemID); serr == nil && len(eps) > 0 {
				return eps, nil
			}
			return nil, err
		}
	}
	return s.st.Episodes(ctx, DefaultProfile, seriesItemID)
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
	case "episode":
		if _, _, err := s.st.EpisodePlayback(ctx, id); err != nil {
			return store.Progress{}, err
		}
	default:
		return store.Progress{}, invalid("tipo non valido: movie o episode")
	}
	p, err := s.st.SaveProgress(ctx, DefaultProfile, media, id, position, duration)
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
	return s.st.GetProgress(ctx, DefaultProfile, media, id)
}

func (s *Service) Continue(ctx context.Context) ([]store.ContinueEntry, error) {
	list, err := s.st.Continue(ctx, DefaultProfile, 30)
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
		title = it.Title
	case "episode":
		var e store.Episode
		var err error
		if it, e, err = s.st.EpisodePlayback(ctx, id); err != nil {
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
		}
	}
	s.vod.sessions[sess.id] = sess
	s.vod.byToken[sess.internal] = sess
	s.vod.mu.Unlock()

	out := PlaybackSession{ID: sess.id, Mode: mode, Title: title, Duration: duration, URL: "/stream/vod/" + sess.id}
	if mode == "remux" {
		probe, err := s.probe(ctx, sess)
		if err != nil {
			return PlaybackSession{}, err
		}
		sess.probe = probe
		if probe.Duration > 0 {
			out.Duration = int(probe.Duration)
		}
		out.URL += "/remux"
	}
	if p, err := s.st.GetProgress(ctx, DefaultProfile, media, id); err == nil && p != nil && !p.Completed && p.Position > 30 {
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

type probeResult struct {
	Duration   float64
	VideoCodec string
	AudioCodec string
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
	var raw struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type  string `json:"codec_type"`
			Codec string `json:"codec_name"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	p := &probeResult{}
	p.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	for _, st := range raw.Streams {
		switch {
		case st.Type == "video" && p.VideoCodec == "":
			p.VideoCodec = st.Codec
		case st.Type == "audio" && p.AudioCodec == "":
			p.AudioCodec = st.Codec
		}
	}
	return p, nil
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
func (s *Service) OpenRemux(ctx context.Context, sessionID string, position float64, force bool) (*Stream, error) {
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
	args = append(args, "-i", "http://"+addr+"/internal/"+sess.internal, "-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn")
	if sess.probe.VideoCodec == "h264" {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p")
	}
	if sess.probe.AudioCodec == "aac" {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "192k")
	}
	args = append(args, "-f", "mp4", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "pipe:1")
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
