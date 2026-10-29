package secret

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSealOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	box, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	sealed := box.Seal([]byte("password"), "subscription:1")
	if bytes.Contains(sealed, []byte("password")) {
		t.Fatal("plaintext visible")
	}
	plain, err := box.Open(sealed, "subscription:1")
	if err != nil || string(plain) != "password" {
		t.Fatalf("open: %q %v", plain, err)
	}
	if _, err := box.Open(sealed, "subscription:2"); !errors.Is(err, ErrCorrupt) {
		t.Fatal("value opened for another owner")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := box.Open(sealed, "subscription:1"); !errors.Is(err, ErrCorrupt) {
		t.Fatal("tampered value accepted")
	}

	again, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := again.Open(box.Seal([]byte("x"), "o"), "o"); err != nil || string(plain) != "x" {
		t.Fatal("reloaded key differs")
	}
}

func TestKeyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if _, err := LoadOrCreate(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(path); err == nil {
		t.Fatal("world-readable key accepted")
	}
}
