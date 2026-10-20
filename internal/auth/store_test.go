package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type memKeyring map[string][]byte

func (m memKeyring) Get(k string) ([]byte, error) {
	v, ok := m[k]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}
func (m memKeyring) Set(k string, v []byte) error { m[k] = v; return nil }
func (m memKeyring) Delete(k string) error        { delete(m, k); return nil }

type session struct{ BDUSS, Cookies string }

func TestRoundTrip(t *testing.T) {
	for _, kr := range []Keyring{memKeyring{}, nil} {
		s := &Store{Dir: t.TempDir(), Keyring: kr}
		var got session
		if err := s.Load(&got); !errors.Is(err, ErrNoSession) {
			t.Fatalf("empty store: %v", err)
		}
		want := session{"bduss", string(make([]byte, 5000))}
		if err := s.Save(want); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(filepath.Join(s.Dir, sessionFile))
		if len(raw) == 0 || string(raw) == "bduss" {
			t.Fatal("session not encrypted")
		}
		if err := s.Load(&got); err != nil || got != want {
			t.Fatalf("load: %v, %+v", err, got.BDUSS)
		}
		if err := s.Clear(); err != nil {
			t.Fatal(err)
		}
		if err := s.Load(&got); !errors.Is(err, ErrNoSession) {
			t.Fatalf("after clear: %v", err)
		}
	}
}

func TestOlderKeyring(t *testing.T) {
	dir := t.TempDir()
	old, cur := memKeyring{}, memKeyring{}
	want := session{"bduss", "c"}
	if err := (&Store{Dir: dir, Keyring: old}).Save(want); err != nil {
		t.Fatal(err)
	}
	s := &Store{Dir: dir, Keyring: cur, Older: []Keyring{memKeyring{}, old}}
	var got session
	if err := s.Load(&got); err != nil || got != want {
		t.Fatalf("load with an older key: %v, %+v", err, got)
	}
	// Saved again under the current key.
	got = session{}
	if err := (&Store{Dir: dir, Keyring: cur}).Load(&got); err != nil || got != want {
		t.Fatalf("not moved to the current key: %v", err)
	}
	if err := (&Store{Dir: dir, Keyring: memKeyring{}}).Load(&got); !errors.Is(err, ErrNoSession) {
		t.Fatalf("a foreign key opened it: %v", err)
	}
}
