package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silirim/vodmax/internal/secret"
	"github.com/silirim/vodmax/internal/store"
)

func TestCommercials(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "vodmax.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	box, _ := secret.LoadOrCreate(filepath.Join(dir, "secret.key"))
	s := New(st, box, nil)
	s.rec.dir = dir
	// A Comskip that finds two breaks (one too short to count) and a
	// chapter mark of another type.
	fake := filepath.Join(dir, "comskip")
	os.WriteFile(fake, []byte(`#!/bin/sh
for a; do f="$a"; done
echo "$@" > `+dir+`/args
printf '0.00\t2.50\t0\n612.40\t842.04\t0\n1500\t1700\t1\n2400.5\t2580\t3\n' > "${f%.*}.edl"
exit 1
`), 0o700)
	s.SetComskip(fake)
	rec := filepath.Join(dir, "7-20261031-2100.mp4")
	os.WriteFile(rec, []byte("video"), 0o600)

	s.findCommercials(rec) // off by default
	if fileExists(edlPath(rec)) {
		t.Fatal("ran while off")
	}
	if c, found, _ := s.CommercialSettings(ctx); c.Enabled || !found {
		t.Fatalf("settings: %+v found %v", c, found)
	}
	s.SetCommercials(ctx, CommercialSettings{Enabled: true})
	os.WriteFile(filepath.Join(dir, "comskip.ini"), []byte("[Main]\n"), 0o600)
	s.findCommercials(rec)
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if !strings.Contains(string(args), "--output="+dir) || !strings.Contains(string(args), "--ini="+dir+"/comskip.ini") {
		t.Fatalf("args: %s", args)
	}
	got := commercials(rec)
	if len(got) != 2 || got[0] != (Commercial{612.4, 842.04}) || got[1] != (Commercial{2400.5, 2580}) {
		t.Fatalf("breaks: %+v", got)
	}
	s.SetComskip(filepath.Join(dir, "missing"))
	if _, found, _ := s.CommercialSettings(ctx); found {
		t.Fatal("missing comskip found")
	}
}
