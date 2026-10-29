package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// A browser reads two far apart regions of a file at once (audio and
// video stored apart): with a single provider connection both must be
// served, with the right bytes.
func TestRangeCacheOneConnectionTwoReaders(t *testing.T) {
	file := make([]byte, 10<<20+12345)
	rand.New(rand.NewSource(1)).Read(file)
	var active, maxActive, opens atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		opens.Add(1)
		for {
			m := maxActive.Load()
			if n <= m || maxActive.CompareAndSwap(m, n) {
				break
			}
		}
		// Like a provider: stops as soon as the client goes.
		var from int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &from)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(file)-1, len(file)))
		w.WriteHeader(http.StatusPartialContent)
		for off := from; off < int64(len(file)); off += 16 << 10 {
			if r.Context().Err() != nil {
				return
			}
			if _, err := w.Write(file[off:min(off+16<<10, int64(len(file)))]); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	var slots atomic.Int32
	c := newRangeCache(t.TempDir(),
		func() (context.Context, func(), error) {
			slots.Add(1)
			ctx, cancel := context.WithCancel(context.Background())
			return ctx, func() { cancel(); slots.Add(-1) }, nil
		},
		func(ctx context.Context, from int64) (io.ReadCloser, int64, error) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
			req.Header.Set("Range", "bytes="+itoa(from)+"-")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, 0, err
			}
			return resp.Body, int64(len(file)), nil
		})
	c.chunk, c.ahead, c.batch, c.budget = 64<<10, 8, 4, 4<<20 // small, to exercise eviction too
	defer c.Close()

	ctx := context.Background()
	if size, err := c.Size(ctx, 0); err != nil || size != int64(len(file)) {
		t.Fatalf("size %d %v", size, err)
	}
	read := func(from, to int64, step int64) []byte {
		var out bytes.Buffer
		for off := from; off <= to; off += step {
			end := min(off+step-1, to)
			if err := c.WriteRange(ctx, &out, off, end); err != nil {
				t.Errorf("range %d-%d: %v", off, end, err)
				return nil
			}
		}
		return out.Bytes()
	}
	var wg sync.WaitGroup
	var a, b []byte
	wg.Add(2)
	go func() { defer wg.Done(); a = read(0, 3<<20, 100_000) }()           // "video", from the start
	go func() { defer wg.Done(); b = read(8<<20, 10<<20+12344, 70_000) }() // "audio", far away, to the end
	wg.Wait()
	if !bytes.Equal(a, file[:3<<20+1]) || !bytes.Equal(b, file[8<<20:]) {
		t.Fatal("wrong bytes")
	}
	if maxActive.Load() > 1 {
		t.Fatalf("%d provider connections at once", maxActive.Load())
	}
	// Seeking back to the start: evicted parts are fetched again, right.
	if got := read(0, 200_000, 50_000); !bytes.Equal(got, file[:200_001]) {
		t.Fatal("wrong bytes after seeking back")
	}
	t.Logf("provider opens: %d", opens.Load())
	if slots.Load() > 1 {
		t.Fatalf("slots held: %d", slots.Load())
	}
}

func itoa(n int64) string { return string(appendInt(nil, n)) }

func appendInt(b []byte, n int64) []byte {
	if n >= 10 {
		b = appendInt(b, n/10)
	}
	return append(b, byte('0'+n%10))
}
