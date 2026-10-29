package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rikimela1992/vodmax/internal/playlist/xtream"
	"github.com/rikimela1992/vodmax/internal/provider"
)

// vodCheck reports container extensions in the catalog and, for a few
// films and one episode per extension, whether the provider honours Range
// requests and which container and video codec the file starts with.
func vodCheck(c *xtream.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	vod, err := c.VODStreams(ctx)
	if err != nil {
		return err
	}
	byExt := map[string][]xtream.VODStream{}
	for _, s := range vod {
		ext := strings.ToLower(s.ContainerExtension.String())
		byExt[ext] = append(byExt[ext], s)
	}
	var exts []string
	for e := range byExt {
		exts = append(exts, e)
	}
	sort.Slice(exts, func(i, j int) bool { return len(byExt[exts[i]]) > len(byExt[exts[j]]) })
	fmt.Printf("film: %d\n", len(vod))
	for _, e := range exts {
		fmt.Printf("  %-6q %6d\n", e, len(byExt[e]))
	}
	for _, e := range exts {
		list := byExt[e]
		for _, i := range []int{0, len(list) / 2, len(list) - 1} {
			s := list[i]
			probeVOD(ctx, c.HTTP(), c.StreamURL(xtream.KindMovie, s.StreamID.String(), e), "film "+e)
			time.Sleep(time.Second)
		}
	}
	series, err := c.Series(ctx)
	if err != nil || len(series) == 0 {
		return err
	}
	for _, idx := range []int{0, len(series) / 2} {
		info, err := c.SeriesInfo(ctx, series[idx].SeriesID.String())
		if err != nil || len(info.Episodes) == 0 {
			continue
		}
		ep := info.Episodes[0]
		probeVOD(ctx, c.HTTP(), c.StreamURL(xtream.KindEpisode, ep.ID.String(), ep.ContainerExtension.String()),
			"episodio "+ep.ContainerExtension.String())
		time.Sleep(time.Second)
	}
	return nil
}

func probeVOD(ctx context.Context, h *http.Client, url, label string) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "VodMax")
	req.Header.Set("Range", "bytes=0-2097151")
	resp, err := h.Do(req)
	if err != nil {
		fmt.Printf("  %-14s errore: %v\n", label, provider.Redact(err))
		return
	}
	defer resp.Body.Close()
	head, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	fmt.Printf("  %-14s HTTP %d in %v, %s, intervallo %q, dimensione %s, %s\n", label, resp.StatusCode,
		time.Since(start).Round(time.Millisecond), resp.Header.Get("Content-Type"), resp.Header.Get("Content-Range"),
		sizeOf(resp), container(head))
}

func sizeOf(resp *http.Response) string {
	cr := resp.Header.Get("Content-Range")
	if i := strings.LastIndex(cr, "/"); i >= 0 {
		return cr[i+1:] + " B"
	}
	return fmt.Sprint(resp.ContentLength) + " B"
}

// container recognises MP4/MKV/TS/AVI and looks for codec markers.
func container(b []byte) string {
	kind := "sconosciuto"
	switch {
	case len(b) > 8 && string(b[4:8]) == "ftyp":
		kind = "MP4"
	case len(b) > 4 && bytes.Equal(b[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}):
		kind = "Matroska/MKV"
	case len(b) > 0 && b[0] == 0x47:
		kind = "MPEG-TS"
	case len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "AVI ":
		kind = "AVI"
	}
	var codecs []string
	for _, m := range []struct{ marker, name string }{
		{"avc1", "H.264"}, {"V_MPEG4/ISO/AVC", "H.264"}, {"hvc1", "HEVC"}, {"hev1", "HEVC"}, {"V_MPEGH/ISO/HEVC", "HEVC"},
		{"mp4a", "AAC"}, {"A_AAC", "AAC"}, {"ac-3", "AC-3"}, {"A_AC3", "AC-3"}, {"ec-3", "E-AC-3"}, {"A_EAC3", "E-AC-3"},
		{"A_DTS", "DTS"}, {"S_TEXT/UTF8", "sottotitoli SRT"}, {"S_HDMV/PGS", "sottotitoli PGS"}, {"XVID", "Xvid"}, {"DX50", "DivX"},
	} {
		if bytes.Contains(b, []byte(m.marker)) && !contains(codecs, m.name) {
			codecs = append(codecs, m.name)
		}
	}
	moovFirst := ""
	if kind == "MP4" {
		if i, j := bytes.Index(b, []byte("moov")), bytes.Index(b, []byte("mdat")); i >= 0 && (j < 0 || i < j) {
			moovFirst = ", indice all'inizio"
		} else {
			moovFirst = ", indice NON all'inizio"
		}
	}
	return kind + " [" + strings.Join(codecs, ", ") + "]" + moovFirst
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
