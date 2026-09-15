package picker

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// fallbackBand is the selected row's background when the terminal does not
// say what its background is: the palette's bright black, which dark themes
// tune as their selection tone.
const fallbackBand = tcell.ColorGray

// How far the band steps from the background it sits on, as a fraction of
// the way to white (dark backgrounds) or black (light ones). A dark theme's
// selection tone sits about a fifth of the way up; a light theme's shaded
// row is subtler, since the eye sees a darkening step sooner.
const (
	darkStep  = 0.22
	lightStep = 0.12
)

// queryTimeout bounds the wait for the terminal's answer. A local terminal
// answers within a few milliseconds; one that never will (Terminal.app, tmux
// without passthrough) costs the picker this much at open.
const queryTimeout = 100 * time.Millisecond

// terminalBand asks the terminal for its background color and derives the
// selected row's band from it, so the band reads as the theme's own
// selection tone on light and dark backgrounds alike. It runs before tcell
// takes the tty. Without an answer it is fallbackBand.
func terminalBand() tcell.Color {
	if bg, ok := terminalBackground(); ok {
		return bandFor(bg)
	}
	return fallbackBand
}

// terminalBackground asks the terminal for its background color over OSC 11
// and reports it, or false when there is no terminal, it does not answer in
// time, or the answer is not a color. The query goes out and the answer is
// read on /dev/tty in raw mode, with select(2) bounding the wait, so a
// silent terminal never blocks the picker. The tty is opened with the raw
// syscall and kept out of Go's poller on purpose: kqueue does not report
// /dev/tty on macOS, so a deadline read there would never wake.
func terminalBackground() (tcell.Color, bool) {
	if t := os.Getenv("TERM"); t == "" || t == "dumb" {
		return 0, false
	}
	fd, err := unix.Open("/dev/tty", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, false
	}
	tty := os.NewFile(uintptr(fd), "/dev/tty")
	defer tty.Close()
	state, err := term.MakeRaw(fd)
	if err != nil {
		return 0, false
	}
	// Best effort: tcell puts the tty in raw mode again a moment later and
	// restores it on exit, so a failed restore here has nothing to leak.
	defer term.Restore(fd, state) //nolint:errcheck
	if _, err := unix.Write(fd, []byte("\x1b]11;?\x1b\\")); err != nil {
		return 0, false
	}
	reply, ok := readReply(fd, time.Now().Add(queryTimeout))
	if !ok {
		return 0, false
	}
	return parseOSC11(reply)
}

// readReply collects bytes from fd until a string terminator (ESC \ or BEL)
// arrives or the deadline passes. It waits for input before every read, so a
// terminal that never answers costs only the wait, not a blocked read.
func readReply(fd int, deadline time.Time) (string, bool) {
	const maxReply = 256
	var reply []byte
	buf := make([]byte, 64)
	for len(reply) < maxReply {
		if !readable(fd, time.Until(deadline)) {
			return "", false
		}
		n, err := unix.Read(fd, buf)
		if err != nil || n == 0 {
			return "", false
		}
		reply = append(reply, buf[:n]...)
		if bytes.HasSuffix(reply, []byte("\x1b\\")) || bytes.HasSuffix(reply, []byte{'\a'}) {
			return string(reply), true
		}
	}
	return "", false
}

// readable waits up to wait for fd to have input. It uses select(2) rather
// than poll(2): on macOS poll does not honour its timeout on a tty, and the
// blocking read that followed would hang the picker on a terminal that never
// answers.
func readable(fd int, wait time.Duration) bool {
	if wait <= 0 {
		return false
	}
	for {
		var set unix.FdSet
		set.Set(fd)
		tv := unix.NsecToTimeval(wait.Nanoseconds())
		n, err := unix.Select(fd+1, &set, nil, nil, &tv)
		if err == unix.EINTR {
			continue
		}
		return err == nil && n > 0
	}
}

// parseOSC11 reads the color out of an OSC 11 answer such as
// "\x1b]11;rgb:2424/2727/3a3a\x1b\\". Terminals answer with one to four hex
// digits per channel, each channel scaled to 8 bits by its own width.
func parseOSC11(reply string) (tcell.Color, bool) {
	_, rest, ok := strings.Cut(reply, "]11;")
	if !ok {
		return 0, false
	}
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, "\x1b\\"), "\a")
	spec, ok := strings.CutPrefix(rest, "rgb:")
	if !ok {
		return 0, false
	}
	parts := strings.Split(spec, "/")
	if len(parts) != 3 {
		return 0, false
	}
	var ch [3]int32
	for i, p := range parts {
		if len(p) == 0 || len(p) > 4 {
			return 0, false
		}
		v, err := strconv.ParseUint(p, 16, 16)
		if err != nil {
			return 0, false
		}
		ch[i] = int32(v * 255 / (1<<(4*len(p)) - 1))
	}
	return tcell.NewRGBColor(ch[0], ch[1], ch[2]), true
}

// bandFor steps the background toward white when it is dark and toward black
// when it is light, by darkStep and lightStep.
func bandFor(bg tcell.Color) tcell.Color {
	r, g, b := bg.RGB()
	toward, step := int32(0), lightStep
	if isDark(r, g, b) {
		toward, step = 255, darkStep
	}
	return tcell.NewRGBColor(lift(r, toward, step), lift(g, toward, step), lift(b, toward, step))
}

// isDark reports whether the color's relative luminance is below the middle.
func isDark(r, g, b int32) bool {
	return (0.299*float64(r)+0.587*float64(g)+0.114*float64(b))/255 < 0.5
}

// lift moves c the given fraction of the way toward target.
func lift(c, target int32, fraction float64) int32 {
	return c + int32(float64(target-c)*fraction)
}
