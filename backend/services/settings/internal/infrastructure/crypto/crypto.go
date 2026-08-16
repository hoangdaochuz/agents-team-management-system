// Package crypto implements the Settings KeyCipher port with AES-GCM sealed by
// a master key derived from SETTINGS_MASTER_KEY (SHA-256). Ciphertext is
// nonce|ciphertext; the master key lives in the process (env), so a DB dump
// leaks nothing usable.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// Cipher is the AES-GCM key cipher adapter.
type Cipher struct {
	aead cipher.AEAD
}

// New derives the 32-byte AES key from the master key and builds the GCM
// cipher. Any passphrase of at least 16 bytes works and the derived key is
// stable across restarts.
func New(masterKey string) (*Cipher, error) {
	if masterKey == "" {
		return nil, errors.New("SETTINGS_MASTER_KEY not set")
	}
	if len(masterKey) < 16 {
		return nil, fmt.Errorf("SETTINGS_MASTER_KEY must be at least 16 bytes")
	}
	sum := sha256.Sum256([]byte(masterKey))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals plaintext as nonce|ciphertext (AES-GCM).
func (c *Cipher) Encrypt(pt []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, pt, nil), nil
}

// Decrypt opens nonce|ciphertext back to plaintext.
func (c *Cipher) Decrypt(ct []byte) ([]byte, error) {
	if len(ct) < c.aead.NonceSize() {
		return nil, errors.New("settings: ciphertext too short")
	}
	nonce, body := ct[:c.aead.NonceSize()], ct[c.aead.NonceSize():]
	return c.aead.Open(nil, nonce, body, nil)
}
