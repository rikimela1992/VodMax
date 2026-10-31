package service

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Video conversion with the graphics card. Only files whose video is not
// H.264 are converted (HEVC, MPEG-2... in the browser); everything else is
// copied. The processor (libx264) is the default and the fallback: when the
// card does not start, the same request is converted by the processor.

const transcodeKey = "transcode"

// TranscodeSettings: Accel is "" (processor), "vaapi" (Intel and AMD),
// "qsv" (Intel Quick Sync) or "nvenc" (NVIDIA); Device is the render node
// for vaapi and qsv.
type TranscodeSettings struct {
	Accel  string `json:"accel"`
	Device string `json:"device"`
}

// Accels are the choices, in the order shown.
var Accels = []string{"", "vaapi", "qsv", "nvenc"}

const defaultRenderNode = "/dev/dri/renderD128"

func (s *Service) TranscodeSettings(ctx context.Context) (TranscodeSettings, error) {
	var t TranscodeSettings
	err := s.st.Setting(ctx, transcodeKey, &t)
	return t, err
}

func (t *TranscodeSettings) check() error {
	t.Device = strings.TrimSpace(t.Device)
	switch t.Accel {
	case "", "nvenc":
		t.Device = ""
	case "vaapi", "qsv":
		if t.Device == "" {
			t.Device = defaultRenderNode
		}
		if !strings.HasPrefix(t.Device, "/dev/dri/") || strings.Contains(t.Device, "..") {
			return invalid("dispositivo non valido: deve essere in /dev/dri/")
		}
	default:
		return invalid("accelerazione sconosciuta: vaapi, qsv o nvenc")
	}
	return nil
}

func (s *Service) SetTranscode(ctx context.Context, t TranscodeSettings) (TranscodeSettings, error) {
	if err := t.check(); err != nil {
		return t, err
	}
	return t, s.st.SetSetting(ctx, transcodeKey, t)
}

// videoArgs are the ffmpeg arguments that convert the video to H.264:
// before goes ahead of the input (devices), after with the output options.
func videoArgs(t TranscodeSettings) (before, after []string) {
	switch t.Accel {
	case "vaapi":
		return []string{"-vaapi_device", t.Device},
			[]string{"-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-qp", "23"}
	case "qsv":
		return []string{"-init_hw_device", "qsv=hw:" + t.Device, "-filter_hw_device", "hw"},
			[]string{"-vf", "format=nv12,hwupload=extra_hw_frames=64", "-c:v", "h264_qsv", "-preset", "veryfast", "-global_quality", "23"}
	case "nvenc":
		return nil, []string{"-c:v", "h264_nvenc", "-preset", "p4", "-rc", "vbr", "-cq", "23", "-pix_fmt", "yuv420p"}
	}
	return nil, []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p"}
}

// TranscodeOption is a choice as the settings show it: whether this
// machine seems to have what it needs.
type TranscodeOption struct {
	Accel  string `json:"accel"`
	Found  bool   `json:"found"`
	Reason string `json:"reason,omitempty"` // no_ffmpeg, no_encoder, no_device
}

// TranscodeOptions looks at the encoders of ffmpeg and at the devices.
func (s *Service) TranscodeOptions(ctx context.Context) ([]TranscodeOption, []string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, s.ffmpegBin(), "-hide_banner", "-encoders").Output()
	encoders := string(out)
	nodes, _ := filepath.Glob("/dev/dri/renderD*")
	_, nvErr := os.Stat("/dev/nvidia0")
	opts := []TranscodeOption{{Accel: "", Found: err == nil}}
	if err != nil {
		opts[0].Reason = "no_ffmpeg"
	}
	for _, a := range Accels[1:] {
		o := TranscodeOption{Accel: a}
		switch {
		case err != nil:
			o.Reason = "no_ffmpeg"
		case !strings.Contains(encoders, " h264_"+a+" "):
			o.Reason = "no_encoder"
		case a != "nvenc" && len(nodes) == 0, a == "nvenc" && nvErr != nil:
			o.Reason = "no_device"
		default:
			o.Found = true
		}
		opts = append(opts, o)
	}
	if nodes == nil {
		nodes = []string{}
	}
	return opts, nodes
}

// TestTranscode converts two seconds of a test picture with the given
// settings and says how it went.
func (s *Service) TestTranscode(ctx context.Context, t TranscodeSettings) (time.Duration, error) {
	if err := t.check(); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	before, after := videoArgs(t)
	args := append([]string{"-nostdin", "-hide_banner", "-loglevel", "error"}, before...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=25:duration=2")
	args = append(args, after...)
	args = append(args, "-f", "null", "-")
	cmd := exec.CommandContext(ctx, s.ffmpegBin(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, n: 4096}
	start := time.Now()
	if err := cmd.Run(); err != nil {
		return 0, invalid("la conversione di prova non è riuscita: %s", firstLine(stderr.String(), err))
	}
	return time.Since(start), nil
}
