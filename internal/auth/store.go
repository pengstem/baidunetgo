// Package auth keeps the signed-in session between runs: encrypted in a
// file of the app's data directory, with the key in the system's credential
// store (whose entries are too small for a browser's cookies).
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrNoSession is what Load returns when nothing was saved.
var ErrNoSession = errors.New("auth: no saved session")

// Keyring holds small secrets, as mygo.Secrets does.
type Keyring interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
}

const (
	keyName     = "session-key"
	sessionFile = "session.enc"
	keyFile     = "session.key" // the key when there is no keyring
)

// Store saves one session in Dir.
type Store struct {
	Dir     string
	Keyring Keyring // nil keeps the key in a file beside the session
}

// Save encrypts v as JSON into the store.
func (s *Store) Save(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	key, err := s.key(true)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := gcm.Seal(nonce, nonce, data, []byte(sessionFile))
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(s.Dir, sessionFile+".tmp")
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.Dir, sessionFile))
}

// Load decrypts the saved session into v.
func (s *Store) Load(v any) error {
	sealed, err := os.ReadFile(filepath.Join(s.Dir, sessionFile))
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNoSession
	}
	if err != nil {
		return err
	}
	key, err := s.key(false)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(sealed) < gcm.NonceSize() {
		return ErrNoSession
	}
	data, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte(sessionFile))
	if err != nil {
		return fmt.Errorf("%w: cannot decrypt", ErrNoSession)
	}
	return json.Unmarshal(data, v)
}

// Clear forgets the session and its key.
func (s *Store) Clear() error {
	err := os.Remove(filepath.Join(s.Dir, sessionFile))
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	if s.Keyring != nil {
		s.Keyring.Delete(keyName)
	}
	os.Remove(filepath.Join(s.Dir, keyFile))
	return err
}

// key returns the encryption key, making one when create is set and
// there is none. It uses the keyring when it works, else a file.
func (s *Store) key(create bool) ([]byte, error) {
	if s.Keyring != nil {
		k, err := s.Keyring.Get(keyName)
		if err == nil && len(k) == 32 {
			return k, nil
		}
		if create {
			k := make([]byte, 32)
			rand.Read(k)
			if s.Keyring.Set(keyName, k) == nil {
				os.Remove(filepath.Join(s.Dir, keyFile))
				return k, nil
			}
		}
	}
	path := filepath.Join(s.Dir, keyFile)
	k, err := os.ReadFile(path)
	if err == nil && len(k) == 32 {
		return k, nil
	}
	if !create {
		return nil, ErrNoSession
	}
	k = make([]byte, 32)
	rand.Read(k)
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, err
	}
	return k, os.WriteFile(path, k, 0o600)
}
