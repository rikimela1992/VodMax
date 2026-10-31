package service

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Advertising breaks in recordings, found by Comskip (an external program,
// used only when installed and turned on). Comskip writes an EDL file next
// to the recording; the player offers to skip each break. Nothing is cut:
// a wrong guess costs a button, not part of the programme.

const commercialsKey = "commercials"

type CommercialSettings struct {
	Enabled bool `json:"enabled"`
}

// comskipMu runs one analysis at a time: it is slow and the machine also
// records and streams.
var comskipMu sync.Mutex

func (s *Service) CommercialSettings(ctx context.Context) (CommercialSettings, bool, error) {
	var c CommercialSettings
	err := s.st.Setting(ctx, commercialsKey, &c)
	_, found := exec.LookPath(s.comskipBin())
	return c, found == nil, err
}

func (s *Service) SetCommercials(ctx context.Context, c CommercialSettings) error {
	return s.st.SetSetting(ctx, commercialsKey, c)
}

func (s *Service) comskipBin() string {
	if s.rec.comskip != "" {
		return s.rec.comskip
	}
	return "comskip"
}

// SetComskip sets the Comskip executable (tests; default: from PATH).
func (s *Service) SetComskip(path string) { s.rec.comskip = path }

func edlPath(file string) string { return strings.TrimSuffix(file, filepath.Ext(file)) + ".edl" }

// findCommercials runs Comskip on a finished recording, when turned on.
func (s *Service) findCommercials(file string) {
	c, found, err := s.CommercialSettings(context.Background())
	if err != nil || !c.Enabled || !found {
		return
	}
	comskipMu.Lock()
	defer comskipMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	args := []string{"--output=" + filepath.Dir(file)}
	if ini := filepath.Join(s.rec.dir, "comskip.ini"); fileExists(ini) {
		args = append(args, "--ini="+ini)
	}
	cmd := exec.CommandContext(ctx, s.comskipBin(), append(args, file)...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &limitedWriter{w: &out, n: 4096}, &limitedWriter{w: &out, n: 4096}
	// Comskip exits with 1 when it finds no break: only a missing EDL is a
	// failure.
	err = cmd.Run()
	if !fileExists(edlPath(file)) && s.rec.log != nil {
		s.rec.log.Warn("pubblicità non analizzata", "file", filepath.Base(file), "err", err, "output", firstLine(out.String(), err))
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Commercial is a break, in seconds from the start of the recording.
type Commercial struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// commercials reads the EDL of a recording: "start end type" per line;
// type 0 (cut) and 3 (commercial break) are breaks.
func commercials(file string) []Commercial {
	f, err := os.Open(edlPath(file))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Commercial
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		start, err1 := strconv.ParseFloat(fields[0], 64)
		end, err2 := strconv.ParseFloat(fields[1], 64)
		if err1 != nil || err2 != nil || end-start < 5 {
			continue
		}
		if len(fields) > 2 && fields[2] != "0" && fields[2] != "3" {
			continue
		}
		out = append(out, Commercial{Start: start, End: end})
	}
	return out
}
