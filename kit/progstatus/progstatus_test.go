package progstatus

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// seq frames a key=value body the way the terminal expects it, so the
// tables below read as the spec writes them.
func seq(body string) string {
	return "\x1b]7501;" + body + "\x1b\\"
}

func TestReports(t *testing.T) {
	tests := []struct {
		name string
		app  string
		emit func(r *Reporter)
		want string
	}{
		{
			name: "working",
			app:  "csl",
			emit: func(r *Reporter) { r.Working("hello") },
			want: seq("state=working:app=csl:msg=aGVsbG8="),
		},
		{
			name: "working without msg",
			app:  "csl",
			emit: func(r *Reporter) { r.Working("") },
			want: seq("state=working:app=csl"),
		},
		{
			name: "progress",
			app:  "csl",
			emit: func(r *Reporter) { r.Progress(42, "hello") },
			want: seq("state=working:progress=42:app=csl:msg=aGVsbG8="),
		},
		{
			name: "progress clamps above 100",
			app:  "csl",
			emit: func(r *Reporter) { r.Progress(150, "") },
			want: seq("state=working:progress=100:app=csl"),
		},
		{
			name: "progress clamps below 0",
			app:  "csl",
			emit: func(r *Reporter) { r.Progress(-5, "") },
			want: seq("state=working:progress=0:app=csl"),
		},
		{
			name: "blocked on permission",
			app:  "csl",
			emit: func(r *Reporter) { r.Blocked(KindPermission, "hello") },
			want: seq("state=blocked:kind=permission:app=csl:msg=aGVsbG8="),
		},
		{
			name: "blocked on a question",
			app:  "csl",
			emit: func(r *Reporter) { r.Blocked(KindQuestion, "") },
			want: seq("state=blocked:kind=question:app=csl"),
		},
		{
			name: "blocked on auth",
			app:  "csl",
			emit: func(r *Reporter) { r.Blocked(KindAuth, "") },
			want: seq("state=blocked:kind=auth:app=csl"),
		},
		{
			name: "blocked without kind",
			app:  "csl",
			emit: func(r *Reporter) { r.Blocked("", "hello") },
			want: seq("state=blocked:app=csl:msg=aGVsbG8="),
		},
		{
			name: "done",
			app:  "csl",
			emit: func(r *Reporter) { r.Done("hello") },
			want: seq("state=done:app=csl:msg=aGVsbG8="),
		},
		{
			name: "done without msg",
			app:  "csl",
			emit: func(r *Reporter) { r.Done("") },
			want: seq("state=done:app=csl"),
		},
		{
			name: "error",
			app:  "csl",
			emit: func(r *Reporter) { r.Error("hello") },
			want: seq("state=error:app=csl:msg=aGVsbG8="),
		},
		{
			name: "error without msg",
			app:  "csl",
			emit: func(r *Reporter) { r.Error("") },
			want: seq("state=error:app=csl"),
		},
		{
			name: "idle",
			app:  "csl",
			emit: func(r *Reporter) { r.Idle() },
			want: seq("state=idle:app=csl"),
		},
		{
			name: "clear",
			app:  "csl",
			emit: func(r *Reporter) { r.Clear() },
			want: seq("state=clear:app=csl"),
		},
		{
			name: "app keeps only the allowed bytes",
			app:  "my tool!/v2.0_a+b-c",
			emit: func(r *Reporter) { r.Idle() },
			want: seq("state=idle:app=mytoolv2.0_a+b-c"),
		},
		{
			name: "app is cut at 32 bytes",
			app:  strings.Repeat("a", 40),
			emit: func(r *Reporter) { r.Idle() },
			want: seq("state=idle:app=" + strings.Repeat("a", 32)),
		},
		{
			name: "app is cut after filtering",
			app:  strings.Repeat("a ", 40),
			emit: func(r *Reporter) { r.Idle() },
			want: seq("state=idle:app=" + strings.Repeat("a", 32)),
		},
		{
			name: "empty app omits the key",
			app:  "",
			emit: func(r *Reporter) { r.Working("hello") },
			want: seq("state=working:msg=aGVsbG8="),
		},
		{
			name: "app with no allowed bytes omits the key",
			app:  "!!! ///",
			emit: func(r *Reporter) { r.Idle() },
			want: seq("state=idle"),
		},
		{
			name: "msg turns control characters into spaces",
			app:  "csl",
			emit: func(r *Reporter) { r.Working("a\x00b\x1b[31mc\x7fd\u0085e\r\n") },
			want: seq("state=working:app=csl:msg=" + b64("a b [31mc d e")),
		},
		{
			name: "msg keeps the word boundaries of a multi-line error",
			app:  "csl",
			emit: func(r *Reporter) { r.Error("denied\nhint: run csl doctor") },
			want: seq("state=error:app=csl:msg=" + b64("denied hint: run csl doctor")),
		},
		{
			name: "msg collapses runs of spaces and trims the ends",
			app:  "csl",
			emit: func(r *Reporter) { r.Working("  a \t\t b  ") },
			want: seq("state=working:app=csl:msg=" + b64("a b")),
		},
		{
			name: "msg made only of control characters is omitted",
			app:  "csl",
			emit: func(r *Reporter) { r.Working("\x07\x1b\n") },
			want: seq("state=working:app=csl"),
		},
		{
			name: "msg keeps non-ASCII text",
			app:  "csl",
			emit: func(r *Reporter) { r.Done("klart: 3 filer ändrade") },
			want: seq("state=done:app=csl:msg=" + b64("klart: 3 filer ändrade")),
		},
		{
			name: "msg with invalid UTF-8 is re-encoded",
			app:  "csl",
			emit: func(r *Reporter) { r.Error("a\xffb") },
			want: seq("state=error:app=csl:msg=" + b64("a�b")),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			tc.emit(NewWriter(&buf, tc.app))
			if got := buf.String(); got != tc.want {
				t.Errorf("report = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMsgTruncation(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "exactly 2048 bytes passes through",
			msg:  strings.Repeat("a", 2048),
			want: strings.Repeat("a", 2048),
		},
		{
			name: "2049 bytes is cut to 2048",
			msg:  strings.Repeat("a", 2049),
			want: strings.Repeat("a", 2048),
		},
		{
			name: "a rune straddling the cap is dropped whole",
			msg:  strings.Repeat("a", 2047) + "é",
			want: strings.Repeat("a", 2047),
		},
		{
			name: "a rune ending at the cap is kept",
			msg:  strings.Repeat("a", 2046) + "éé",
			want: strings.Repeat("a", 2046) + "é",
		},
		{
			name: "leading control characters vanish before the cap applies",
			msg:  strings.Repeat("\x00", 10) + strings.Repeat("a", 2048),
			want: strings.Repeat("a", 2048),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			NewWriter(&buf, "csl").Working(tc.msg)
			want := seq("state=working:app=csl:msg=" + b64(tc.want))
			if got := buf.String(); got != want {
				t.Errorf("report = %q, want %q", got, want)
			}
		})
	}
}

// TestLongestReportFitsSpecCap pins the reason encode carries no length
// guard: the biggest report the API can produce stays under 4096 bytes.
func TestLongestReportFitsSpecCap(t *testing.T) {
	var buf bytes.Buffer
	NewWriter(&buf, strings.Repeat("a", 40)).Blocked(KindPermission, strings.Repeat("é", 2000))
	if buf.Len() > 4096 {
		t.Errorf("longest report is %d bytes, want at most 4096", buf.Len())
	}
}

func TestNoOps(t *testing.T) {
	var nilReporter *Reporter
	var zero Reporter
	reporters := map[string]*Reporter{
		"nil reporter":  nilReporter,
		"zero reporter": &zero,
		"nil writer":    NewWriter(nil, "csl"),
	}
	for name, r := range reporters {
		t.Run(name, func(t *testing.T) {
			r.Working("x")
			r.Progress(50, "x")
			r.Blocked(KindQuestion, "x")
			r.Done("x")
			r.Error("x")
			r.Idle()
			r.Clear()
			ran := false
			err := r.Run(context.Background(), "x", func() error {
				ran = true
				return errors.New("boom")
			})
			if !ran {
				t.Error("Run did not call fn")
			}
			if err == nil || err.Error() != "boom" {
				t.Errorf("Run returned %v, want fn's error", err)
			}
		})
	}
}

func TestRun(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, expire := context.WithTimeout(context.Background(), 0)
	t.Cleanup(expire)
	boom := errors.New("boom")

	tests := []struct {
		name    string
		ctx     context.Context
		fnErr   error
		wantErr error
		want    string
	}{
		{
			name: "nil error reports done with the same msg",
			ctx:  context.Background(),
			want: seq(
				"state=working:app=csl:msg=aGVsbG8=",
			) + seq(
				"state=done:app=csl:msg=aGVsbG8=",
			),
		},
		{
			name:    "context.Canceled reports idle",
			ctx:     context.Background(),
			fnErr:   context.Canceled,
			wantErr: context.Canceled,
			want:    seq("state=working:app=csl:msg=aGVsbG8=") + seq("state=idle:app=csl"),
		},
		{
			name:    "a wrapped cancellation reports idle",
			ctx:     context.Background(),
			fnErr:   errors.Join(errors.New("stopped"), context.Canceled),
			wantErr: context.Canceled,
			want:    seq("state=working:app=csl:msg=aGVsbG8=") + seq("state=idle:app=csl"),
		},
		{
			name:    "any error on a canceled ctx reports idle",
			ctx:     canceled,
			fnErr:   boom,
			wantErr: boom,
			want:    seq("state=working:app=csl:msg=aGVsbG8=") + seq("state=idle:app=csl"),
		},
		{
			name:    "a deadline that ran out reports error, not idle",
			ctx:     expired,
			fnErr:   context.DeadlineExceeded,
			wantErr: context.DeadlineExceeded,
			want: seq("state=working:app=csl:msg=aGVsbG8=") +
				seq("state=error:app=csl:msg=Y29udGV4dCBkZWFkbGluZSBleGNlZWRlZA=="),
		},
		{
			name:    "other errors report error with the message",
			ctx:     context.Background(),
			fnErr:   boom,
			wantErr: boom,
			want: seq(
				"state=working:app=csl:msg=aGVsbG8=",
			) + seq(
				"state=error:app=csl:msg=Ym9vbQ==",
			),
		},
		{
			name:    "nil ctx is tolerated",
			fnErr:   boom,
			wantErr: boom,
			want: seq(
				"state=working:app=csl:msg=aGVsbG8=",
			) + seq(
				"state=error:app=csl:msg=Ym9vbQ==",
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := NewWriter(&buf, "csl").Run(tc.ctx, "hello", func() error { return tc.fnErr })
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Run returned %v, want %v", err, tc.wantErr)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("reports = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStepsWritesOncePerPercent(t *testing.T) {
	var buf bytes.Buffer
	const total = 250
	steps := NewWriter(&buf, "csl").Steps(total)
	for i := 1; i <= total; i++ {
		steps.Step(i, "repo")
	}
	if got := strings.Count(buf.String(), "\x1b]7501;"); got != 100 {
		t.Fatalf("reports = %d, want 100 (one per percent)", got)
	}
	last := seq("state=working:progress=100:app=csl:msg=" + b64("repo"))
	if !strings.HasSuffix(buf.String(), last) {
		t.Fatalf("last report is not progress=100: %q", buf.String())
	}
}

func TestStepsSkipsRepeatsAndZero(t *testing.T) {
	tests := []struct {
		name  string
		total int
		steps []int
		want  int
	}{
		{name: "zero percent", total: 10, steps: []int{0}, want: 0},
		{name: "zero total", total: 0, steps: []int{1}, want: 0},
		{name: "same step twice", total: 4, steps: []int{1, 1}, want: 1},
		{name: "different steps on the same percent", total: 200, steps: []int{2, 3}, want: 1},
		{name: "complete", total: 4, steps: []int{4}, want: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			steps := NewWriter(&buf, "csl").Steps(tc.total)
			for _, i := range tc.steps {
				steps.Step(i, "repo")
			}
			if got := strings.Count(buf.String(), "\x1b]7501;"); got != tc.want {
				t.Errorf("reports = %d, want %d: %q", got, tc.want, buf.String())
			}
		})
	}
}

func TestStepsNoOps(t *testing.T) {
	var nilReporter *Reporter
	var nilSteps *Steps
	nilReporter.Steps(3).Step(1, "x")
	nilSteps.Step(1, "x")
	NewWriter(nil, "csl").Steps(3).Step(3, "x")
}

// TestNewDiscardsUnderGoTest pins the contract that a test run never
// writes escape bytes to stderr, whether or not stderr is a terminal.
func TestNewDiscardsUnderGoTest(t *testing.T) {
	r := New("csl")
	if r == nil {
		t.Fatal("New returned nil, want a discarding Reporter")
	}
	if r.w != nil {
		t.Errorf("New under go test writes to %T, want a discarding Reporter", r.w)
	}
	if r.app != "csl" {
		t.Errorf("New kept app %q, want %q", r.app, "csl")
	}
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
