package envdefault

import (
	"strings"
	"testing"
	"time"
)

const varName = "THISMOON_TEST_VALUE"

func TestString(t *testing.T) {
	tests := []struct {
		name     string
		set      bool
		value    string
		fallback string
		want     string
	}{
		{name: "unset falls back", fallback: "7423", want: "7423"},
		{
			name:     "set wins",
			set:      true,
			value:    "http://csl.this",
			fallback: "x",
			want:     "http://csl.this",
		},
		{name: "empty falls back", set: true, value: "", fallback: "x", want: "x"},
		{name: "whitespace is a value", set: true, value: " ", fallback: "x", want: " "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(varName, tc.value)
			}
			if got := String(varName, tc.fallback); got != tc.want {
				t.Errorf("String(%q, %q) = %q, want %q", varName, tc.fallback, got, tc.want)
			}
		})
	}
}

func TestInt(t *testing.T) {
	tests := []struct {
		name     string
		set      bool
		value    string
		fallback int
		want     int
		wantWarn string
	}{
		{name: "unset falls back", fallback: 7423, want: 7423},
		{name: "set wins", set: true, value: "9999", fallback: 7423, want: 9999},
		{name: "empty falls back", set: true, value: "", fallback: 7423, want: 7423},
		{name: "negative parses", set: true, value: "-1", fallback: 7423, want: -1},
		{
			name: "unparseable warns and falls back", set: true, value: "nope", fallback: 7423,
			want:     7423,
			wantWarn: `THISMOON_TEST_VALUE: unparseable value "nope", using default 7423`,
		},
		{
			name: "float warns and falls back", set: true, value: "74.23", fallback: 7423,
			want:     7423,
			wantWarn: `THISMOON_TEST_VALUE: unparseable value "74.23", using default 7423`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(varName, tc.value)
			}
			var warn strings.Builder
			got := intFrom(&warn, varName, tc.fallback)
			if got != tc.want {
				t.Errorf("Int(%q, %d) = %d, want %d", varName, tc.fallback, got, tc.want)
			}
			if tc.wantWarn == "" {
				if warn.Len() != 0 {
					t.Errorf(
						"Int(%q, %d) warned %q, want silence",
						varName,
						tc.fallback,
						warn.String(),
					)
				}
				return
			}
			if want := tc.wantWarn + "\n"; warn.String() != want {
				t.Errorf("warning = %q, want %q", warn.String(), want)
			}
		})
	}
}

func TestBool(t *testing.T) {
	tests := []struct {
		name     string
		set      bool
		value    string
		fallback bool
		want     bool
		wantWarn string
	}{
		{name: "unset falls back", fallback: true, want: true},
		{name: "true wins", set: true, value: "true", fallback: false, want: true},
		{name: "1 wins", set: true, value: "1", fallback: false, want: true},
		{name: "FALSE wins", set: true, value: "FALSE", fallback: true, want: false},
		{name: "empty falls back", set: true, value: "", fallback: true, want: true},
		{
			name: "unparseable warns and falls back", set: true, value: "yes", fallback: false,
			want:     false,
			wantWarn: `THISMOON_TEST_VALUE: unparseable value "yes", using default false`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(varName, tc.value)
			}
			var warn strings.Builder
			got := boolFrom(&warn, varName, tc.fallback)
			if got != tc.want {
				t.Errorf("Bool(%q, %t) = %t, want %t", varName, tc.fallback, got, tc.want)
			}
			if tc.wantWarn == "" {
				if warn.Len() != 0 {
					t.Errorf(
						"Bool(%q, %t) warned %q, want silence",
						varName,
						tc.fallback,
						warn.String(),
					)
				}
				return
			}
			if want := tc.wantWarn + "\n"; warn.String() != want {
				t.Errorf("warning = %q, want %q", warn.String(), want)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name     string
		set      bool
		value    string
		fallback time.Duration
		want     time.Duration
		wantWarn string
	}{
		{name: "unset falls back", fallback: 10 * time.Minute, want: 10 * time.Minute},
		{name: "set wins", set: true, value: "90s", fallback: time.Minute, want: 90 * time.Second},
		{name: "empty falls back", set: true, value: "", fallback: time.Minute, want: time.Minute},
		{
			name: "unparseable warns and falls back", set: true, value: "soon", fallback: time.Minute,
			want:     time.Minute,
			wantWarn: `THISMOON_TEST_VALUE: unparseable value "soon", using default 1m0s`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(varName, tc.value)
			}
			var warn strings.Builder
			got := durationFrom(&warn, varName, tc.fallback)
			if got != tc.want {
				t.Errorf("Duration(%q, %s) = %s, want %s", varName, tc.fallback, got, tc.want)
			}
			if tc.wantWarn == "" {
				if warn.Len() != 0 {
					t.Errorf("Duration warned %q, want silence", warn.String())
				}
				return
			}
			if want := tc.wantWarn + "\n"; warn.String() != want {
				t.Errorf("warning = %q, want %q", warn.String(), want)
			}
		})
	}
}
