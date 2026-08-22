package event

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		ev      Event
		wantErr bool
	}{
		{"ok", Event{Source: "deps", Title: "x", Level: "warn"}, false},
		{"empty level ok", Event{Source: "deps", Title: "x"}, false},
		{"missing source", Event{Title: "x"}, true},
		{"blank source", Event{Source: "   ", Title: "x"}, true},
		{"missing title", Event{Source: "deps"}, true},
		{"blank title", Event{Source: "deps", Title: "  "}, true},
		{"bad level", Event{Source: "deps", Title: "x", Level: "fatal"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ev.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeLevel(t *testing.T) {
	if got := NormalizeLevel(""); got != LevelInfo {
		t.Errorf("NormalizeLevel(\"\") = %q, want %q", got, LevelInfo)
	}
	if got := NormalizeLevel("error"); got != "error" {
		t.Errorf("NormalizeLevel(error) = %q, want error", got)
	}
}

func TestSanitizeSource(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"deps", "deps", false},
		{"MyApp", "myapp", false},
		{"  sandbox watch  ", "sandbox-watch", false},
		{"a//b", "a-b", false},
		{"a---b", "a-b", false},
		{"--lead-trail--", "lead-trail", false},
		{"keep_under-score", "keep_under-score", false},
		{"日本語", "", true},
		{"###", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := SanitizeSource(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SanitizeSource(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("SanitizeSource(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeSourceCapsLength(t *testing.T) {
	long := bytes.Repeat([]byte("a"), 200)
	got, err := SanitizeSource(string(long))
	if err != nil {
		t.Fatalf("SanitizeSource long: %v", err)
	}
	if len(got) > maxSourceLen {
		t.Errorf("sanitized length = %d, want <= %d", len(got), maxSourceLen)
	}
}

// zeroReader yields deterministic bytes so IDs differ only by their timestamp.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestNewIDOrdering(t *testing.T) {
	r := zeroReader{}
	t1 := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Nanosecond)
	id1 := NewID(t1, r)
	id2 := NewID(t2, r)
	if id1 >= id2 {
		t.Errorf("later time should yield lexically greater id: %q >= %q", id1, id2)
	}
	if len(id1) != len(id2) {
		t.Errorf("ids should be fixed width: %d vs %d", len(id1), len(id2))
	}
}

func TestTimeFromID(t *testing.T) {
	stamp := time.Date(2026, 8, 22, 10, 30, 0, 123, time.UTC)
	id := NewID(stamp, strings.NewReader("ab"))

	got, ok := TimeFromID(id)
	if !ok {
		t.Fatalf("TimeFromID(%q) not ok", id)
	}
	if !got.Equal(stamp) {
		t.Errorf("TimeFromID = %v, want %v", got, stamp)
	}

	for _, bad := range []string{"", "short", "not-a-number-prefix-x"} {
		if _, ok := TimeFromID(bad); ok {
			t.Errorf("TimeFromID(%q) ok = true, want false", bad)
		}
	}
}
