package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rikimela1992/vodmax/internal/playlist/xtream"
	"github.com/rikimela1992/vodmax/internal/provider"
)

// liveCheck opens a few live channels for a few seconds each, reporting
// start-up time and codecs, then inspects the provider's XMLTV guide.
// Nothing is stored. Each stream uses the account's connection only while
// it is being read.
func liveCheck(c *xtream.Client, names []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	streams, err := c.LiveStreams(ctx)
	if err != nil {
		return err
	}
	withEPG := 0
	epgIDs := map[string]bool{}
	for _, s := range streams {
		if id := strings.TrimSpace(s.EPGChannelID.String()); id != "" {
			withEPG++
			epgIDs[strings.ToLower(id)] = true
		}
	}
	fmt.Printf("canali: %d, con epg_channel_id: %d (%d distinti)\n", len(streams), withEPG, len(epgIDs))

	for _, name := range names {
		var found *xtream.LiveStream
		for i := range streams {
			if strings.EqualFold(strings.TrimSpace(streams[i].Name.String()), name) {
				found = &streams[i]
				break
			}
		}
		if found == nil {
			fmt.Printf("\n%s: non trovato\n", name)
			continue
		}
		fmt.Printf("\n%s (epg %q):\n", name, found.EPGChannelID)
		for _, ext := range []string{"ts", "m3u8"} {
			probeStream(ctx, c.HTTP(), c.StreamURL(xtream.KindLive, found.StreamID.String(), ext), ext)
			time.Sleep(time.Second) // let the provider release the connection
		}
	}
	return xmltvCheck(ctx, c, epgIDs)
}

func probeStream(ctx context.Context, h *http.Client, url, ext string) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "VodMax")
	resp, err := h.Do(req)
	if err != nil {
		fmt.Printf("  %-4s errore: %v\n", ext, provider.Redact(err))
		return
	}
	defer resp.Body.Close()
	headers := time.Since(start)
	if ext == "m3u8" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		lines := 0
		for _, l := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(l, "#EXT") {
				lines++
			}
		}
		fmt.Printf("  m3u8 HTTP %d %s in %v, %d righe #EXT, inizia con %q\n", resp.StatusCode,
			resp.Header.Get("Content-Type"), headers.Round(time.Millisecond), lines, firstLine(body))
		return
	}
	buf := make([]byte, 0, 4<<20)
	chunk := make([]byte, 64<<10)
	var firstByte time.Duration
	deadline := time.Now().Add(6 * time.Second)
	for len(buf) < cap(buf) && time.Now().Before(deadline) {
		n, err := resp.Body.Read(chunk)
		if n > 0 && firstByte == 0 {
			firstByte = time.Since(start)
		}
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
	}
	elapsed := time.Since(start) - firstByte
	kbps := 0.0
	if elapsed > 0 {
		kbps = float64(len(buf)) * 8 / 1000 / elapsed.Seconds()
	}
	fmt.Printf("  ts   HTTP %d %s, intestazioni %v, primo byte %v, %d KB letti (~%.0f kbit/s), codec %s\n",
		resp.StatusCode, resp.Header.Get("Content-Type"), headers.Round(time.Millisecond),
		firstByte.Round(time.Millisecond), len(buf)/1024, kbps, tsCodecs(buf))
}

func firstLine(b []byte) string {
	line, _, _ := strings.Cut(string(b), "\n")
	if len(line) > 40 {
		line = line[:40]
	}
	return line
}

// tsCodecs reads PAT and PMT of an MPEG-TS sample and names the streams.
func tsCodecs(b []byte) string {
	names := map[byte]string{0x01: "MPEG-1", 0x02: "MPEG-2", 0x03: "MP3", 0x04: "MP2", 0x0f: "AAC", 0x11: "AAC-LATM",
		0x1b: "H.264", 0x24: "HEVC", 0x81: "AC-3", 0x87: "E-AC-3", 0x06: "privato(AC-3/sottotitoli)"}
	pmtPIDs := map[uint16]bool{}
	var out []string
	seen := map[string]bool{}
	for i := 0; i+188 <= len(b); i += 188 {
		if b[i] != 0x47 {
			// resync
			j := i + 1
			for j < len(b) && b[j] != 0x47 {
				j++
			}
			i = j - 188
			continue
		}
		pid := uint16(b[i+1]&0x1f)<<8 | uint16(b[i+2])
		if b[i+1]&0x40 == 0 {
			continue
		}
		payload := b[i+4 : i+188]
		if b[i+3]&0x20 != 0 {
			payload = payload[1+int(payload[0]):]
		}
		if len(payload) < 1 {
			continue
		}
		payload = payload[1+int(payload[0]):] // pointer field
		if len(payload) < 12 {
			continue
		}
		secLen := int(payload[1]&0x0f)<<8 | int(payload[2])
		if 3+secLen > len(payload) {
			continue
		}
		if pid == 0 && payload[0] == 0x00 {
			for k := 8; k+4 <= 3+secLen-4; k += 4 {
				if prog := uint16(payload[k])<<8 | uint16(payload[k+1]); prog != 0 {
					pmtPIDs[uint16(payload[k+2]&0x1f)<<8|uint16(payload[k+3])] = true
				}
			}
		} else if pmtPIDs[pid] && payload[0] == 0x02 {
			infoLen := int(payload[10]&0x0f)<<8 | int(payload[11])
			for k := 12 + infoLen; k+5 <= 3+secLen-4; {
				st := payload[k]
				esLen := int(payload[k+3]&0x0f)<<8 | int(payload[k+4])
				name := names[st]
				if name == "" {
					name = fmt.Sprintf("0x%02x", st)
				}
				if !seen[name] {
					seen[name] = true
					out = append(out, name)
				}
				k += 5 + esLen
			}
			break
		}
	}
	if len(out) == 0 {
		return "non rilevati"
	}
	return strings.Join(out, " + ")
}

func xmltvCheck(ctx context.Context, c *xtream.Client, epgIDs map[string]bool) error {
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.XMLTVURL(), nil)
	req.Header.Set("User-Agent", "VodMax")
	resp, err := c.HTTP().Do(req)
	if err != nil {
		return fmt.Errorf("xmltv: %w", provider.Redact(err))
	}
	defer resp.Body.Close()
	counter := &countingReader{r: resp.Body}
	dec := xml.NewDecoder(counter)
	dec.Strict = false
	channels := map[string]bool{}
	programmes := map[string]int{}
	var minStart, maxStop string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("\nxmltv: errore di lettura dopo %d KB: %v\n", counter.n/1024, err)
			break
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attr := func(name string) string {
			for _, a := range el.Attr {
				if a.Name.Local == name {
					return a.Value
				}
			}
			return ""
		}
		switch el.Name.Local {
		case "channel":
			channels[strings.ToLower(attr("id"))] = true
		case "programme":
			programmes[strings.ToLower(attr("channel"))]++
			if s := attr("start"); minStart == "" || s < minStart {
				minStart = s
			}
			if s := attr("stop"); s > maxStop {
				maxStop = s
			}
		}
	}
	covered := 0
	for id := range epgIDs {
		if programmes[id] > 0 {
			covered++
		}
	}
	total := 0
	for _, n := range programmes {
		total += n
	}
	fmt.Printf("\nxmltv: HTTP %d, %d MB in %v, %d canali, %d programmi su %d canali, intervallo %s → %s\n",
		resp.StatusCode, counter.n>>20, time.Since(start).Round(time.Second), len(channels), total, len(programmes), minStart, maxStop)
	fmt.Printf("epg_channel_id dei flussi con programmi nella guida: %d su %d\n", covered, len(epgIDs))
	return nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
