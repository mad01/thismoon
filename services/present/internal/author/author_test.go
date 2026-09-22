package author

import (
	"errors"
	"net/http"
	"regexp"
	"testing"
)

func bearer(key string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+key)
	return h
}

func TestNewKeyIs64HexAndUnique(t *testing.T) {
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	a, b := NewKey(), NewKey()
	if !hex64.MatchString(a) || !hex64.MatchString(b) {
		t.Fatalf("keys %q %q are not 64 hex chars", a, b)
	}
	if a == b {
		t.Fatal("two keys collided")
	}
}

func TestHashIsStableAndNotTheKey(t *testing.T) {
	key := NewKey()
	first := Hash(key)
	if Hash(key) != first {
		t.Fatal("Hash is not deterministic")
	}
	if Hash(key) == key {
		t.Fatal("Hash returned the key itself")
	}
	if Hash("a") == Hash("b") {
		t.Fatal("distinct keys hashed alike")
	}
}

func TestFromHeader(t *testing.T) {
	key := NewKey()
	cases := []struct {
		name   string
		header http.Header
		wantOK bool
	}{
		{"nil header", nil, false},
		{"absent", http.Header{}, false},
		{"bearer", bearer(key), true},
		{"bearer lowercase scheme", http.Header{"Authorization": {"bearer " + key}}, true},
		{"basic scheme", http.Header{"Authorization": {"Basic abc"}}, false},
		{"bearer without token", http.Header{"Authorization": {"Bearer "}}, false},
		{"bare token", http.Header{"Authorization": {key}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hash, ok := FromHeader(tc.header)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && hash != Hash(key) {
				t.Fatalf("hash = %q, want Hash(key)", hash)
			}
			if !ok && hash != "" {
				t.Fatalf("hash = %q on failure, want empty", hash)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	key, other := NewKey(), NewKey()
	owner := Hash(key)
	if err := Check(owner, bearer(key)); err != nil {
		t.Errorf("owner key: %v, want nil", err)
	}
	if err := Check(owner, bearer(other)); !errors.Is(err, ErrMismatch) {
		t.Errorf("other key: %v, want ErrMismatch", err)
	}
	if err := Check(owner, http.Header{}); !errors.Is(err, ErrMissing) {
		t.Errorf("no key: %v, want ErrMissing", err)
	}
	if err := Check("", bearer(key)); !errors.Is(err, ErrMismatch) {
		t.Errorf("authorless page: %v, want ErrMismatch", err)
	}
}
