package hint

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

var day1 = time.Date(2026, 11, 14, 9, 0, 0, 0, time.Local)

// fakeBrain writes a brain stand-in that records its arguments and prints
// body as its listing, exiting with code. No vault is read.
func fakeBrain(t *testing.T, dir, body string, code int) string {
	t.Helper()
	p := filepath.Join(dir, "brain")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" >> " + filepath.Join(dir, "calls") + "\n" +
		"echo '" + body + "'\n" +
		"exit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func calls(t *testing.T, dir string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "calls"))
	return string(b)
}

func TestLine(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		code    int
		want    string // prefix of the line; "" means silence
		wantErr bool
	}{
		{name: "empty inbox", body: `{"tier":"inbox","total":0,"notes":null}`},
		{
			name: "three captures",
			body: `{"tier":"inbox","total":3,"notes":null}`,
			want: "3 harvest captures from earlier sessions",
		},
		{
			name: "one capture",
			body: `{"tier":"inbox","total":1,"notes":null}`,
			want: "1 harvest capture from earlier sessions",
		},
		{name: "no total", body: `{"tier":"inbox","notes":null}`},
		{name: "brain fails", body: `{"tier":"inbox","total":3}`, code: 1, wantErr: true},
		{name: "not JSON", body: "vault offline", wantErr: true},
		{name: "a total that is not a count", body: `{"total":2.5}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st := state.New(filepath.Join(dir, "state"), time.Now)
			cfg := Config{
				State: st, Brain: fakeBrain(t, dir, tc.body, tc.code),
				Now: func() time.Time { return day1 },
			}
			got, err := Line(context.Background(), cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Line() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.want == "" && got != "" {
				t.Errorf("Line() = %q, want silence", got)
			}
			if tc.want != "" && !strings.HasPrefix(got, tc.want) {
				t.Errorf("Line() = %q, want it to lead with %q", got, tc.want)
			}
			stamp, _ := st.Stamp()
			if spoke := got != ""; spoke != (stamp != "") {
				t.Errorf("spoke = %v but stamp = %q: the stamp must follow the line", spoke, stamp)
			}
		})
	}
}

func TestLineAsksBrainForRawHarvestCaptures(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		State: state.New(filepath.Join(dir, "state"), time.Now),
		Brain: fakeBrain(t, dir, `{"total":0}`, 0),
		Now:   func() time.Time { return day1 },
	}
	if _, err := Line(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	want := "list\ninbox\n--tag\nharvest\n--status\nraw\n-o\njson\n"
	if got := calls(t, dir); got != want {
		t.Errorf("brain called with %q, want %q", got, want)
	}
}

func TestLineSpeaksOnceADay(t *testing.T) {
	dir := t.TempDir()
	now := day1
	cfg := Config{
		State: state.New(filepath.Join(dir, "state"), time.Now),
		Brain: fakeBrain(t, dir, `{"total":3}`, 0),
		Now:   func() time.Time { return now },
	}
	if got, _ := Line(context.Background(), cfg); got == "" {
		t.Fatal("the hint stayed silent with captures waiting")
	}
	if got, err := Line(context.Background(), cfg); got != "" || err != nil {
		t.Errorf("second Line() the same day = %q, %v; want silence", got, err)
	}
	if n := strings.Count(calls(t, dir), "list\n"); n != 1 {
		t.Errorf("brain called %d times, want once: a stamped day asks nothing", n)
	}
	now = day1.Add(24 * time.Hour)
	if got, _ := Line(context.Background(), cfg); got == "" {
		t.Error("the hint stayed silent on the next day")
	}
}

func TestLineMissingBrain(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		State: state.New(filepath.Join(dir, "state"), time.Now),
		Brain: filepath.Join(dir, "no-such-brain"),
		Now:   func() time.Time { return day1 },
	}
	if got, err := Line(context.Background(), cfg); got != "" || err == nil {
		t.Errorf("Line() without brain = %q, %v; want silence and an error", got, err)
	}
}

func TestMessage(t *testing.T) {
	want := "3 harvest captures from earlier sessions wait in the brain inbox, unverified, " +
		"and each expires 30 days after it was written. Tell the user this once, in one line. " +
		"`brain list inbox --tag harvest` lists them, `brain promote <note>` keeps one for good, " +
		"and `brain inbox keep <note>` gives one more time."
	if got := Message(3); got != want {
		t.Errorf("Message(3) =\n%s\nwant\n%s", got, want)
	}
}
