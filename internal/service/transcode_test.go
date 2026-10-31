package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEncoder is an ffmpeg whose graphics card never starts: it fails with
// a hardware encoder and writes its arguments otherwise.
func fakeEncoder(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	path := filepath.Join(dir, "ffmpeg")
	script := `#!/bin/sh
echo "$@" >> ` + log + `
case "$*" in *h264_vaapi*|*h264_nvenc*|*h264_qsv*) echo "No VA display found" >&2; exit 1;; esac
case "$*" in *-encoders*) printf ' V....D h264_vaapi           H.264 (VAAPI)\n V....D libx264 x\n';; *) printf 'ftypisom-video';; esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	s.SetFFmpeg(path, "")
	return s, log
}

func TestRemuxFallsBackToProcessor(t *testing.T) {
	s, log := fakeEncoder(t)
	ctx := context.Background()
	input := []string{"-nostdin", "-i", "http://x/in"}
	out := []string{"-c:a", "copy", "-f", "mp4", "pipe:1"}
	body, err := s.remux(ctx, input, out, TranscodeSettings{Accel: "vaapi", Device: "/dev/dri/renderD128"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(body)
	body.Close()
	if string(got) != "ftypisom-video" {
		t.Fatalf("body %q", got)
	}
	raw, _ := os.ReadFile(log)
	calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "-vaapi_device /dev/dri/renderD128 -nostdin") ||
		!strings.Contains(calls[0], "h264_vaapi") || !strings.Contains(calls[1], "libx264") || strings.Contains(calls[1], "vaapi") {
		t.Fatalf("calls:\n%s", raw)
	}
	// H.264 is copied: no card, no second try.
	os.Remove(log)
	body, err = s.remux(ctx, input, out, TranscodeSettings{Accel: "vaapi", Device: "/dev/dri/renderD128"}, true, "")
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	if raw, _ := os.ReadFile(log); strings.Count(string(raw), "\n") != 1 || !strings.Contains(string(raw), "-c:v copy") || strings.Contains(string(raw), "vaapi") {
		t.Fatalf("copy: %s", raw)
	}
}

func TestTranscodeChoices(t *testing.T) {
	s, _ := fakeEncoder(t)
	ctx := context.Background()
	if _, err := s.TestTranscode(ctx, TranscodeSettings{Accel: "vaapi"}); err == nil || !strings.Contains(err.Error(), "No VA display") {
		t.Fatalf("test with a missing card: %v", err)
	}
	if _, err := s.TestTranscode(ctx, TranscodeSettings{}); err != nil {
		t.Fatalf("processor: %v", err)
	}
	for _, bad := range []TranscodeSettings{{Accel: "cuda"}, {Accel: "vaapi", Device: "/etc/passwd"}, {Accel: "qsv", Device: "/dev/dri/../sda"}} {
		if err := bad.check(); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	ok := TranscodeSettings{Accel: "qsv"}
	if err := ok.check(); err != nil || ok.Device != defaultRenderNode {
		t.Fatalf("default device: %+v %v", ok, err)
	}
	opts, _ := s.TranscodeOptions(ctx)
	if len(opts) != len(Accels) || !opts[0].Found || opts[3].Found || opts[3].Reason != "no_encoder" {
		t.Fatalf("options: %+v", opts)
	}
}
