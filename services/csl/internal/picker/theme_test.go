package picker

import (
	"os"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestParseOSC11(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  tcell.Color
		ok    bool
	}{
		{
			name:  "sixteen bit, ST terminated",
			reply: "\x1b]11;rgb:2424/2727/3a3a\x1b\\",
			want:  tcell.NewRGBColor(0x24, 0x27, 0x3a),
			ok:    true,
		},
		{
			name:  "sixteen bit, BEL terminated",
			reply: "\x1b]11;rgb:ffff/ffff/ffff\a",
			want:  tcell.NewRGBColor(255, 255, 255),
			ok:    true,
		},
		{
			name:  "eight bit",
			reply: "\x1b]11;rgb:1e/20/30\x1b\\",
			want:  tcell.NewRGBColor(0x1e, 0x20, 0x30),
			ok:    true,
		},
		{
			name:  "four bit scales up",
			reply: "\x1b]11;rgb:f/0/8\x1b\\",
			want:  tcell.NewRGBColor(255, 0, 0x88),
			ok:    true,
		},
		{
			name:  "keystroke before the answer",
			reply: "j\x1b]11;rgb:0000/0000/0000\x1b\\",
			want:  tcell.NewRGBColor(0, 0, 0),
			ok:    true,
		},
		{name: "empty", reply: ""},
		{name: "not a color", reply: "\x1b]11;?\x1b\\"},
		{name: "wrong channel count", reply: "\x1b]11;rgb:00/00\x1b\\"},
		{name: "bad hex", reply: "\x1b]11;rgb:zz/00/00\x1b\\"},
		{name: "channel too wide", reply: "\x1b]11;rgb:00000/00/00\x1b\\"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseOSC11(tt.reply)
			if ok != tt.ok {
				t.Fatalf("parseOSC11(%q) ok = %v, want %v", tt.reply, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("parseOSC11(%q) = %v, want %v", tt.reply, got, tt.want)
			}
		})
	}
}

func TestBandFor(t *testing.T) {
	tests := []struct {
		name string
		bg   tcell.Color
		want tcell.Color
	}{
		{
			name: "dark navy steps toward white",
			bg:   tcell.NewRGBColor(0x24, 0x27, 0x3a),
			want: tcell.NewRGBColor(84, 86, 101),
		},
		{
			name: "black steps toward white",
			bg:   tcell.NewRGBColor(0, 0, 0),
			want: tcell.NewRGBColor(56, 56, 56),
		},
		{
			name: "cream steps toward black",
			bg:   tcell.NewRGBColor(0xfd, 0xf6, 0xe3),
			want: tcell.NewRGBColor(223, 217, 200),
		},
		{
			name: "white steps toward black",
			bg:   tcell.NewRGBColor(255, 255, 255),
			want: tcell.NewRGBColor(225, 225, 225),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bandFor(tt.bg); got != tt.want {
				gr, gg, gb := got.RGB()
				wr, wg, wb := tt.want.RGB()
				t.Errorf("bandFor = (%d,%d,%d), want (%d,%d,%d)", gr, gg, gb, wr, wg, wb)
			}
		})
	}
}

// TestReadReply drives the read loop over a pipe: a complete answer comes
// back, a partial one times out, and silence times out without blocking.
func TestReadReply(t *testing.T) {
	const timeout = 50 * time.Millisecond
	tests := []struct {
		name  string
		write string
		want  string
		ok    bool
	}{
		{
			name:  "complete answer",
			write: "\x1b]11;rgb:00/00/00\x1b\\",
			want:  "\x1b]11;rgb:00/00/00\x1b\\",
			ok:    true,
		},
		{
			name:  "answer in two writes",
			write: "\x1b]11;rgb:00/00/00\a",
			want:  "\x1b]11;rgb:00/00/00\a",
			ok:    true,
		},
		{name: "no terminator times out", write: "\x1b]11;rgb:00"},
		{name: "silence times out", write: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe: %v", err)
			}
			defer r.Close()
			defer w.Close()
			if tt.write != "" {
				if _, err := w.WriteString(tt.write); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			start := time.Now()
			got, ok := readReply(int(r.Fd()), start.Add(timeout))
			if ok != tt.ok || got != tt.want {
				t.Errorf("readReply = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
			if !tt.ok && time.Since(start) < timeout {
				t.Errorf(
					"readReply returned after %v, before the %v deadline",
					time.Since(start),
					timeout,
				)
			}
			if time.Since(start) > timeout+200*time.Millisecond {
				t.Errorf("readReply took %v, well past the %v deadline", time.Since(start), timeout)
			}
		})
	}
}

// TestTerminalBackgroundLive runs the real query against the controlling
// terminal, so it needs one that answers OSC 11 and only runs when
// PICKER_TTY_PROBE is set. A manual check drives it under a pty that answers
// with a known color and reads the logged result.
func TestTerminalBackgroundLive(t *testing.T) {
	if os.Getenv("PICKER_TTY_PROBE") == "" {
		t.Skip("set PICKER_TTY_PROBE=1 under a terminal that answers OSC 11")
	}
	start := time.Now()
	bg, ok := terminalBackground()
	r, g, b := bg.RGB()
	t.Logf("terminalBackground() = (%d,%d,%d), %v after %v", r, g, b, ok, time.Since(start))
	if !ok {
		t.Fatal("no answer from the terminal")
	}
}
