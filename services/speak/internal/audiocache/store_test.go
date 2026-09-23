package audiocache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/speak/internal/tts"
)

func TestKeyShapeAndInputs(t *testing.T) {
	base := Key("local af_heart", 0, "Hello there.")
	if !ValidKey(base) {
		t.Fatalf("Key = %q, want 32 lowercase hex characters", base)
	}
	if again := Key("local af_heart", 0, "Hello there."); again != base {
		t.Errorf("Key is not stable: %q then %q", base, again)
	}
	for name, other := range map[string]string{
		"clip":  Key("local am_adam", 0, "Hello there."),
		"speed": Key("local af_heart", 1.25, "Hello there."),
		"text":  Key("local af_heart", 0, "Hello there!"),
		// The clip ID's length prefix keeps a shifted boundary apart.
		"boundary": Key("local af_heart\n0", 0, "\nHello there."),
	} {
		if other == base {
			t.Errorf("changing the %s left the key unchanged", name)
		}
	}
}

func TestValidKey(t *testing.T) {
	cases := map[string]bool{
		"0123456789abcdef0123456789abcdef":  true,
		"0123456789ABCDEF0123456789ABCDEF":  false, // uppercase
		"0123456789abcdef0123456789abcde":   false, // short
		"0123456789abcdef0123456789abcdef0": false, // long
		"../../../../etc/passwd0123456789a": false,
		"0123456789abcdef0123456789abcde/":  false,
		"":                                  false,
	}
	for key, want := range cases {
		if got := ValidKey(key); got != want {
			t.Errorf("ValidKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestStorePutGetRoundTrip(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "cache")) // Put creates the directory
	wav := tts.Audio{Data: tts.WAV([]byte{1, 2, 3, 4}, 24000, 1), ContentType: tts.ContentTypeWAV}
	mp3 := tts.Audio{Data: []byte("ID3fake"), ContentType: tts.ContentTypeMP3}
	for _, tc := range []struct {
		key   string
		audio tts.Audio
	}{
		{Key("a", 0, "wav clip"), wav},
		{Key("a", 0, "mp3 clip"), mp3},
	} {
		if s.Has(tc.key) {
			t.Fatalf("Has(%s) before Put = true", tc.key)
		}
		if err := s.Put(tc.key, tc.audio); err != nil {
			t.Fatalf("Put: %v", err)
		}
		got, ok, err := s.Get(tc.key)
		if err != nil || !ok {
			t.Fatalf("Get = ok %v, err %v; want the stored clip", ok, err)
		}
		if string(got.Data) != string(tc.audio.Data) || got.ContentType != tc.audio.ContentType {
			t.Errorf("Get = %q %s, want %q %s", got.Data, got.ContentType,
				tc.audio.Data, tc.audio.ContentType)
		}
	}
	if _, ok, err := s.Get(Key("a", 0, "never stored")); ok || err != nil {
		t.Errorf("Get of a missing clip = ok %v, err %v; want a plain miss", ok, err)
	}
}

func TestStoreRejectsInvalidKeys(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "cache"))
	bad := "../escape0123456789abcdef012345"
	if err := s.Put(bad, tts.Audio{Data: []byte("RIFF")}); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("Put(%q) err = %v, want ErrInvalidKey", bad, err)
	}
	if _, _, err := s.Get(bad); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("Get(%q) err = %v, want ErrInvalidKey", bad, err)
	}
	if s.Has(bad) || s.Touch(bad) {
		t.Errorf("Has/Touch accepted %q", bad)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("an invalid key wrote %d entries next to the cache", len(entries))
	}
}

func TestStoreUseBumpsMtime(t *testing.T) {
	s := NewStore(t.TempDir())
	key := Key("a", 0, "touched")
	if err := s.Put(key, tts.Audio{Data: []byte("RIFF")}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.dir, key+".wav")
	old := time.Now().Add(-48 * time.Hour)
	for name, use := range map[string]func() bool{
		"Touch": func() bool { return s.Touch(key) },
		"Get": func() bool {
			_, ok, err := s.Get(key)
			return ok && err == nil
		},
	} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		if !use() {
			t.Fatalf("%s did not find the clip", name)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if time.Since(info.ModTime()) > time.Hour {
			t.Errorf("%s left mtime at %v, want it bumped to now", name, info.ModTime())
		}
	}
	if s.Touch(Key("a", 0, "missing")) {
		t.Error("Touch of a missing clip = true")
	}
}

func TestStoreReapByMtime(t *testing.T) {
	s := NewStore(t.TempDir())
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	stale, fresh := Key("a", 0, "stale"), Key("a", 0, "fresh")
	for _, key := range []string{stale, fresh} {
		if err := s.Put(key, tts.Audio{Data: []byte("RIFF")}); err != nil {
			t.Fatal(err)
		}
	}
	strayTemp := filepath.Join(s.dir, stale+".123.tmp")
	unrelated := filepath.Join(s.dir, "notes.txt")
	for _, path := range []string{strayTemp, unrelated} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ages := map[string]time.Duration{
		filepath.Join(s.dir, stale+".wav"): 31 * 24 * time.Hour,
		filepath.Join(s.dir, fresh+".wav"): 29 * 24 * time.Hour,
		strayTemp:                          31 * 24 * time.Hour,
		unrelated:                          365 * 24 * time.Hour,
	}
	for path, age := range ages {
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}

	reaped, err := s.Reap(30*24*time.Hour, 1<<30, now)
	if err != nil || reaped != (Reaped{Expired: 2}) {
		t.Fatalf("Reap = %+v, %v; want the stale clip and the stray temp file removed", reaped,
			err)
	}
	if s.Has(stale) || !s.Has(fresh) {
		t.Errorf("after Reap: stale kept %v, fresh kept %v; want only fresh", s.Has(stale),
			s.Has(fresh))
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("Reap touched a file that is not a clip: %v", err)
	}
}

// TestStoreReapEvictsOverCap pins the size cap: past it, the least recently
// used clips go first, and a temporary file Put may still be writing is
// neither counted nor removed.
func TestStoreReapEvictsOverCap(t *testing.T) {
	s := NewStore(t.TempDir())
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	const clipBytes = 100
	var keys []string
	for i := range 4 {
		key := Key("a", 0, fmt.Sprintf("clip %d", i))
		keys = append(keys, key)
		if err := s.Put(key, tts.Audio{Data: make([]byte, clipBytes)}); err != nil {
			t.Fatal(err)
		}
		used := now.Add(-time.Duration(4-i) * time.Hour) // clip 0 is the least recently used
		if err := os.Chtimes(filepath.Join(s.dir, key+".wav"), used, used); err != nil {
			t.Fatal(err)
		}
	}
	writing := filepath.Join(s.dir, keys[0]+".456.tmp")
	if err := os.WriteFile(writing, make([]byte, 10*clipBytes), 0o644); err != nil {
		t.Fatal(err)
	}

	reaped, err := s.Reap(24*time.Hour, 2*clipBytes+clipBytes/2, now)
	if err != nil || reaped != (Reaped{Evicted: 2}) {
		t.Fatalf("Reap = %+v, %v; want the two least recently used clips evicted", reaped, err)
	}
	for i, key := range keys {
		if want := i >= 2; s.Has(key) != want {
			t.Errorf("clip %d kept = %v, want %v", i, s.Has(key), want)
		}
	}
	if _, err := os.Stat(writing); err != nil {
		t.Errorf("Reap removed a temporary file within maxAge: %v", err)
	}
}

func TestStoreReapMissingDir(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "never-created"))
	if reaped, err := s.Reap(time.Hour, 0, time.Now()); reaped != (Reaped{}) || err != nil {
		t.Errorf("Reap of a missing dir = %+v, %v; want nothing, nil", reaped, err)
	}
}
