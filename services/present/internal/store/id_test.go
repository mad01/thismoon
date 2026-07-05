package store

import (
	"regexp"
	"testing"
)

func TestNewIDFormat(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{10}$`)
	for range 100 {
		id := NewID()
		if !re.MatchString(id) {
			t.Fatalf("NewID() = %q, want 10 lowercase hex chars", id)
		}
	}
}

func TestNewIDUnique(t *testing.T) {
	seen := make(map[string]struct{}, 10000)
	for range 10000 {
		id := NewID()
		if _, dup := seen[id]; dup {
			t.Fatalf("NewID() produced duplicate %q", id)
		}
		seen[id] = struct{}{}
	}
}
