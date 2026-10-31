package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Updates: once a day the server looks at the project's GitHub releases
// (and nothing else: no data about the installation is sent) and keeps
// the newest one. The interface shows it to the admin; the Android app
// uses it to update itself. It can be turned off.

const (
	updateRepo  = "silirim/VodMax"
	updateEvery = 24 * time.Hour
	updateRetry = 3 * time.Hour // after a failed check
	updatesKey  = "updates"
	updateKey   = "update_info"
)

// ReleaseAsset is a downloadable file of a release.
type ReleaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

// UpdateInfo is what the server knows about new versions.
type UpdateInfo struct {
	Enabled    bool          `json:"enabled"`
	Beta       bool          `json:"beta"`                 // betas are offered too
	Prerelease bool          `json:"prerelease,omitempty"` // Latest is a beta
	Current    string        `json:"current"`
	Latest     string        `json:"latest,omitempty"`
	Available  bool          `json:"available"` // newer than this server
	Notes      string        `json:"notes,omitempty"`
	Page       string        `json:"page,omitempty"`
	Published  *time.Time    `json:"published_at,omitempty"`
	APK        *ReleaseAsset `json:"apk,omitempty"`
	Server     *ReleaseAsset `json:"server,omitempty"` // for this machine's architecture
	CheckedAt  *time.Time    `json:"checked_at,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// Betas can be installed by anyone (from the release page or with
// vodmax-update <tag>), but are offered only with Beta on.
type updateSettings struct {
	Enabled bool `json:"enabled"`
	Beta    bool `json:"beta"`
}

type updateState struct {
	mu      sync.Mutex
	version string
	api     string // GitHub API base (tests point it elsewhere)
	next    time.Time
}

// SetVersion tells the service its own version ("v0.3.1", "0.3.1-2-g…",
// "dev").
func (s *Service) SetVersion(v string) {
	s.upd.mu.Lock()
	s.upd.version = v
	s.upd.mu.Unlock()
}

// SetUpdateAPI points the release check elsewhere (tests).
func (s *Service) SetUpdateAPI(base string) {
	s.upd.mu.Lock()
	s.upd.api = base
	s.upd.mu.Unlock()
}

func (s *Service) updateSettings(ctx context.Context) updateSettings {
	set := updateSettings{Enabled: true}
	s.st.Setting(ctx, updatesKey, &set)
	return set
}

// SetUpdatesEnabled turns the daily check on or off; beta (when given)
// chooses whether betas are offered, and asks GitHub again.
func (s *Service) SetUpdatesEnabled(ctx context.Context, on bool, beta *bool) (UpdateInfo, error) {
	set := s.updateSettings(ctx)
	set.Enabled = on
	changed := beta != nil && *beta != set.Beta
	if beta != nil {
		set.Beta = *beta
	}
	if err := s.st.SetSetting(ctx, updatesKey, set); err != nil {
		return UpdateInfo{}, err
	}
	if changed && on {
		info, _ := s.CheckUpdate(ctx)
		return info, nil
	}
	return s.Update(ctx), nil
}

// Update returns the last known release information.
func (s *Service) Update(ctx context.Context) UpdateInfo {
	var info UpdateInfo
	s.st.Setting(ctx, updateKey, &info)
	s.upd.mu.Lock()
	info.Current = s.upd.version
	s.upd.mu.Unlock()
	set := s.updateSettings(ctx)
	info.Enabled, info.Beta = set.Enabled, set.Beta
	info.Available = info.Latest != "" && newerVersion(info.Latest, info.Current)
	return info
}

// CheckUpdate asks GitHub now and stores the result.
func (s *Service) CheckUpdate(ctx context.Context) (UpdateInfo, error) {
	info, err := s.fetchUpdate(ctx)
	now := time.Now()
	if err != nil {
		old := s.Update(ctx)
		old.Error, old.CheckedAt = T(ctx, "controllo degli aggiornamenti non riuscito: %v", err), &now
		s.st.SetSetting(context.WithoutCancel(ctx), updateKey, old)
		return s.Update(ctx), err
	}
	info.CheckedAt = &now
	if err := s.st.SetSetting(ctx, updateKey, info); err != nil {
		return UpdateInfo{}, err
	}
	return s.Update(ctx), nil
}

// updateDue runs the daily check from the maintenance loop.
func (s *Service) updateDue(ctx context.Context, log *slog.Logger, now time.Time) {
	s.upd.mu.Lock()
	wait, version := now.Before(s.upd.next), s.upd.version
	s.upd.mu.Unlock()
	// Development builds ("dev") and tests have no version to compare.
	if parseVersion(version) == nil || !s.updateSettings(ctx).Enabled {
		return
	}
	if wait {
		return
	}
	last := s.Update(ctx)
	if last.CheckedAt != nil && now.Sub(*last.CheckedAt) < updateEvery && last.Error == "" {
		s.upd.mu.Lock()
		s.upd.next = last.CheckedAt.Add(updateEvery)
		s.upd.mu.Unlock()
		return
	}
	next := now.Add(updateEvery)
	info, err := s.CheckUpdate(ctx)
	if err != nil {
		next = now.Add(updateRetry)
		log.Warn("controllo degli aggiornamenti non riuscito", "err", err)
	} else if info.Available {
		log.Info("nuova versione disponibile", "versione", info.Latest)
	}
	s.upd.mu.Lock()
	s.upd.next = next
	s.upd.mu.Unlock()
}

type ghRelease struct {
	Tag        string    `json:"tag_name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Body       string    `json:"body"`
	Page       string    `json:"html_url"`
	Published  time.Time `json:"published_at"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

const maxNotes = 4000

func (s *Service) fetchUpdate(ctx context.Context) (UpdateInfo, error) {
	s.upd.mu.Lock()
	base, version := s.upd.api, s.upd.version
	s.upd.mu.Unlock()
	if base == "" {
		base = "https://api.github.com"
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var releases []ghRelease
	if err := s.getJSON(ctx, base+"/repos/"+updateRepo+"/releases?per_page=20", version, &releases); err != nil {
		return UpdateInfo{}, err
	}
	// Betas only for whoever asked for them. Someone on a beta without the
	// option is offered the next stable version.
	beta := s.updateSettings(ctx).Beta
	var best *ghRelease
	for i := range releases {
		r := &releases[i]
		if r.Draft || parseVersion(r.Tag) == nil || (r.Prerelease && !beta) {
			continue
		}
		if best == nil || newerVersion(r.Tag, best.Tag) {
			best = r
		}
	}
	if best == nil {
		return UpdateInfo{}, errors.New("nessuna versione pubblicata")
	}
	info := UpdateInfo{Latest: best.Tag, Notes: best.Body, Page: best.Page, Prerelease: best.Prerelease}
	if len(info.Notes) > maxNotes {
		info.Notes = info.Notes[:maxNotes] + "…"
	}
	if !best.Published.IsZero() {
		p := best.Published
		info.Published = &p
	}
	arch := map[string]string{"amd64": "amd64", "arm64": "arm64", "arm": "armv7"}[runtime.GOARCH]
	var sums string
	for _, a := range best.Assets {
		asset := &ReleaseAsset{Name: a.Name, URL: a.URL, Size: a.Size}
		switch {
		case a.Name == "SHA256SUMS":
			sums = a.URL
		case strings.HasSuffix(a.Name, ".apk"):
			info.APK = asset
		case arch != "" && strings.HasSuffix(a.Name, "_linux_"+arch+".tar.gz"):
			info.Server = asset
		}
	}
	// Downloads are checked against the release's checksums: without them
	// the app is not offered.
	if sums == "" {
		info.APK, info.Server = nil, nil
		return info, nil
	}
	hashes, err := s.fetchSums(ctx, sums, version)
	if err != nil {
		return UpdateInfo{}, err
	}
	for _, a := range []*ReleaseAsset{info.APK, info.Server} {
		if a != nil {
			a.SHA256 = hashes[a.Name]
		}
	}
	if info.APK != nil && info.APK.SHA256 == "" {
		info.APK = nil
	}
	if info.Server != nil && info.Server.SHA256 == "" {
		info.Server = nil
	}
	return info, nil
}

func (s *Service) getJSON(ctx context.Context, url, version string, v any) error {
	resp, err := s.directGet(ctx, url, version)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}

func (s *Service) directGet(ctx context.Context, url, version string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "VodMax/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.directHTTP().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("risposta HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

var sumLine = regexp.MustCompile(`^([0-9a-f]{64})\s+\*?(\S+)$`)

func (s *Service) fetchSums(ctx context.Context, url, version string) (map[string]string, error) {
	resp, err := s.directGet(ctx, url, version)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 64<<10))
	for sc.Scan() {
		if m := sumLine.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
			out[m[2]] = m[1]
		}
	}
	return out, sc.Err()
}

// parseVersion reads "v1.2.3", "1.2.3", "v1.2.3-4-gabc" (git describe) or
// "v1.2.3-rc1"; nil for anything else ("dev").
var (
	versionRE   = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-(.+))?$`)
	describeRE  = regexp.MustCompile(`^\d+-g[0-9a-f]+(-dirty)?$`)
	firstNumber = regexp.MustCompile(`\d+`)
)

func parseVersion(v string) []int {
	m := versionRE.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return nil
	}
	out := make([]int, 4)
	for i := 0; i < 3; i++ {
		out[i], _ = strconv.Atoi(m[i+1])
	}
	// A pre-release ("-beta.2", "-rc1") comes before the release, in the
	// order of its number; a build after a tag ("-4-gabc") after it.
	switch {
	case m[4] == "":
	case describeRE.MatchString(m[4]):
		out[3] = 1
	default:
		n, _ := strconv.Atoi(firstNumber.FindString(m[4]))
		out[3] = -1000 + min(n, 999)
	}
	return out
}

// newerVersion: a is a later version than b; false if either is unknown.
func newerVersion(a, b string) bool {
	x, y := parseVersion(a), parseVersion(b)
	if x == nil || y == nil {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

func (s *Service) version() string {
	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	return s.upd.version
}
