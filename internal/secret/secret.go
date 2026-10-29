// Package secret encrypts credentials at rest (VPN private keys, provider
// passwords, playlist URLs) with a per-installation key.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Box seals and opens values with AES-256-GCM. The associated data binds a
// sealed value to its owner (e.g. "vpn:3"), so values cannot be swapped
// between rows.
type Box struct{ aead cipher.AEAD }

const keySize = 32

// LoadOrCreate reads the key file, creating it with mode 0600 on first use.
// A key file readable by group or others is refused.
func LoadOrCreate(path string) (*Box, error) {
	key, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		key = make([]byte, keySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(key); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s: permessi troppo aperti (%v), servono 0600", path, info.Mode().Perm())
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("%s: chiave di lunghezza non valida", path)
	}
	return New(key)
}

// New builds a Box from a raw 32-byte key.
func New(key []byte) (*Box, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal returns nonce || ciphertext.
func (b *Box) Seal(plain []byte, owner string) []byte {
	nonce := make([]byte, b.aead.NonceSize(), b.aead.NonceSize()+len(plain)+b.aead.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return b.aead.Seal(nonce, nonce, plain, []byte(owner))
}

// ErrCorrupt means the value was altered, sealed for another owner, or
// sealed with another installation's key.
var ErrCorrupt = errors.New("segreto non decifrabile")

// Open reverses Seal.
func (b *Box) Open(sealed []byte, owner string) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n+b.aead.Overhead() {
		return nil, ErrCorrupt
	}
	plain, err := b.aead.Open(nil, sealed[:n], sealed[n:], []byte(owner))
	if err != nil {
		return nil, ErrCorrupt
	}
	return plain, nil
}
