package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/mad01/thismoon/services/present/internal/store"
	"github.com/mad01/thismoon/services/present/internal/terminal"
)

// terminalDeadline bounds every socket exchange; generous because a
// process launch on a loaded machine can take seconds.
const terminalDeadline = 30 * time.Second

type terminalFixture struct {
	ts     *httptest.Server
	mgr    *terminal.Manager
	page   string
	toggle string
}

// setupTerminal serves one page with a terminal manager whose sessions
// start in a temp home, with the switch on or off.
func setupTerminal(t *testing.T, mode Mode, on bool) *terminalFixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("store.NewFS: %v", err)
	}
	p, err := st.Create(t.Context(), store.Draft{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	home := t.TempDir()
	mgr := terminal.New(terminal.Config{
		Home:      home,
		Env:       []string{"HOME=" + home, "PATH=/usr/bin:/bin"},
		KillGrace: 500 * time.Millisecond,
	})
	t.Cleanup(mgr.Close)
	toggle := terminal.SwitchPath(dir)
	if on {
		if err := terminal.Enable(toggle); err != nil {
			t.Fatal(err)
		}
	}
	srv := New(st, Options{
		Mode: mode, Workdir: dir, Info: testInfo, Terminals: mgr, TerminalSwitch: toggle,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &terminalFixture{ts: ts, mgr: mgr, page: p.ID, toggle: toggle}
}

func (f *terminalFixture) socketURL(session string, q url.Values) string {
	u := "ws" + strings.TrimPrefix(f.ts.URL, "http") + "/api/p/" + f.page + "/terminal/" + session
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (f *terminalFixture) dial(
	t *testing.T, session string, q url.Values, opts *websocket.DialOptions,
) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), terminalDeadline)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, f.socketURL(session, q), opts)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if conn != nil {
		t.Cleanup(func() { _ = conn.CloseNow() })
	}
	return conn, resp, err
}

func (f *terminalFixture) mustDial(t *testing.T, session string, q url.Values) *websocket.Conn {
	t.Helper()
	conn, _, err := f.dial(t, session, q, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", session, err)
	}
	return conn
}

// terminalReader reads one socket, keeping the binary output it has seen.
type terminalReader struct {
	conn   *websocket.Conn
	output bytes.Buffer
}

// until reads frames until a text frame of kind arrives and returns it,
// collecting the output on the way. A close first fails the test.
func (r *terminalReader) until(t *testing.T, kind string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), terminalDeadline)
	defer cancel()
	for {
		typ, data, err := r.conn.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %q: %v (output %q)", kind, err, r.output.String())
		}
		if typ == websocket.MessageBinary {
			r.output.Write(data)
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("text frame %q: %v", data, err)
		}
		if msg["t"] == kind {
			return msg
		}
	}
}

// outputUntil reads frames until want has arrived in the output.
func (r *terminalReader) outputUntil(t *testing.T, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), terminalDeadline)
	defer cancel()
	for !strings.Contains(r.output.String(), want) {
		typ, data, err := r.conn.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %q: %v (output %q)", want, err, r.output.String())
		}
		if typ == websocket.MessageBinary {
			r.output.Write(data)
		}
	}
}

// closeStatus reads until the socket closes and returns the close status.
func (r *terminalReader) closeStatus(t *testing.T) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), terminalDeadline)
	defer cancel()
	for {
		if _, _, err := r.conn.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func writeText(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(t.Context(), websocket.MessageText, raw); err != nil {
		t.Fatalf("write control: %v", err)
	}
}

func TestTerminalOffAnswers404ButListsDisabled(t *testing.T) {
	f := setupTerminal(t, ModeLocal, false)

	code, body := get(t, f.ts.URL+"/api/p/"+f.page+"/terminal/demo")
	if code != http.StatusNotFound || !strings.Contains(body, terminalOffBody) {
		t.Errorf("socket route off = %d %q, want 404 naming the enable command", code, body)
	}
	code, body = get(t, f.ts.URL+"/api/terminal")
	if code != http.StatusOK {
		t.Fatalf("GET /api/terminal = %d, want 200 while off", code)
	}
	var list struct {
		Enabled  bool              `json:"enabled"`
		Sessions []json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("list body %q: %v", body, err)
	}
	if list.Enabled || list.Sessions == nil || len(list.Sessions) != 0 {
		t.Errorf("list = %s, want enabled false and an empty sessions array", body)
	}
}

func TestTerminalRefusesBadNamesAndMissingPages(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true)
	cases := []struct {
		path string
		want int
	}{
		{"/api/p/" + f.page + "/terminal/Bad_Name", http.StatusBadRequest},
		{"/api/p/" + f.page + "/terminal/" + strings.Repeat("a", 33), http.StatusBadRequest},
		{"/api/p/missing0000/terminal/demo", http.StatusNotFound},
		{"/api/p/" + f.page + "/terminal/demo?cols=0", http.StatusBadRequest},
		{"/api/p/" + f.page + "/terminal/demo?rows=wide", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if code, body := get(t, f.ts.URL+tc.path); code != tc.want {
			t.Errorf("GET %s = %d %q, want %d", tc.path, code, body, tc.want)
		}
	}
	if got := f.mgr.List(); len(got) != 0 {
		t.Errorf("refused requests spawned %+v", got)
	}
}

func TestTerminalRefusesForeignOrigins(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true)
	host := strings.TrimPrefix(f.ts.URL, "http://")
	_, port, _ := strings.Cut(host, ":")
	cases := []struct {
		name string
		opts *websocket.DialOptions
	}{
		{"another site", &websocket.DialOptions{
			HTTPHeader: http.Header{"Origin": {"http://evil.example"}},
		}},
		{"another local port", &websocket.DialOptions{
			HTTPHeader: http.Header{"Origin": {"http://localhost:1"}},
		}},
		{"rebound DNS name agreeing with Host", &websocket.DialOptions{
			Host:       "evil.example:" + port,
			HTTPHeader: http.Header{"Origin": {"http://evil.example:" + port}},
		}},
	}
	q := url.Values{"cmd": {"sleep 60"}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, resp, err := f.dial(t, "demo", q, tc.opts)
			if err == nil {
				t.Fatal("dial succeeded, want it refused")
			}
			if resp == nil || resp.StatusCode != http.StatusForbidden {
				t.Errorf("dial response = %v, want 403", resp)
			}
		})
	}
	if got := f.mgr.List(); len(got) != 0 {
		t.Errorf("refused origins spawned %+v", got)
	}
}

// d-man proxies present.this with the Host rewritten to the loopback
// address serve listens on, so the request Host never equals the Origin
// there. The present.this pattern is what admits it, and only it.
func TestTerminalAcceptsTheDManOriginOnALoopbackHost(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true)
	loopback := strings.TrimPrefix(f.ts.URL, "http://")
	if !strings.HasPrefix(loopback, "127.0.0.1:") {
		t.Fatalf("test server on %s, want a 127.0.0.1 address", loopback)
	}
	dialFrom := func(origin string) (*websocket.Conn, *http.Response, error) {
		return f.dial(t, "demo", url.Values{"cmd": {"printf hi; cat"}}, &websocket.DialOptions{
			Host:       loopback,
			HTTPHeader: http.Header{"Origin": {origin}},
		})
	}

	_, resp, err := dialFrom("http://evil.example")
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("evil.example on a loopback Host: err %v, response %v; want 403", err, resp)
	}
	conn, _, err := dialFrom("http://present.this")
	if err != nil {
		t.Fatalf("present.this on a loopback Host refused: %v", err)
	}
	r := &terminalReader{conn: conn}
	r.until(t, "hello")
	r.outputUntil(t, "hi")
}

func TestTerminalSocketSendsHelloOutputThenExit(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true)
	r := &terminalReader{conn: f.mustDial(t, "demo", url.Values{"cmd": {"printf hi; exit 4"}})}

	hello := r.until(t, "hello")
	if hello["replayed"] != float64(0) {
		t.Errorf("hello = %v, want nothing replayed on a fresh session", hello)
	}
	exit := r.until(t, "exit")
	if exit["code"] != float64(4) {
		t.Errorf("exit = %v, want code 4", exit)
	}
	if !strings.Contains(r.output.String(), "hi") {
		t.Errorf("output = %q, want hi", r.output.String())
	}
	if st := r.closeStatus(t); st != websocket.StatusNormalClosure {
		t.Errorf("close status = %v, want normal closure", st)
	}
}

func TestTerminalReattachReplaysThenTakesInputAndKill(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true)
	q := url.Values{"cmd": {"printf ready; cat"}}
	first := &terminalReader{conn: f.mustDial(t, "demo", q)}
	first.outputUntil(t, "ready")
	_ = first.conn.Close(websocket.StatusNormalClosure, "tab reloaded")

	again := &terminalReader{conn: f.mustDial(t, "demo", url.Values{"cmd": {"ignored"}})}
	hello := again.until(t, "hello")
	if n, _ := hello["replayed"].(float64); n < float64(len("ready")) {
		t.Errorf("hello = %v, want the earlier output replayed", hello)
	}
	again.outputUntil(t, "ready")
	if err := again.conn.Write(t.Context(), websocket.MessageBinary, []byte("ping\n")); err != nil {
		t.Fatalf("write input: %v", err)
	}
	again.outputUntil(t, "ping")

	writeText(t, again.conn, map[string]any{"t": "resize", "cols": 100, "rows": 30})
	writeText(t, again.conn, map[string]string{"t": "kill"})
	if exit := again.until(t, "exit"); exit["code"] == float64(0) {
		t.Errorf("exit after kill = %v, want a signal code", exit)
	}
	if got := f.mgr.List(); len(got) != 0 {
		t.Errorf("List() after kill = %+v", got)
	}
}

func TestTerminalDeleteKillsTheSession(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true)
	r := &terminalReader{conn: f.mustDial(t, "demo", url.Values{"cmd": {"printf up; sleep 60"}})}
	r.outputUntil(t, "up")

	if code := deleteTerminal(t, f, "demo"); code != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204", code)
	}
	r.until(t, "exit")
	if code := deleteTerminal(t, f, "demo"); code != http.StatusNotFound {
		t.Errorf("second DELETE = %d, want 404", code)
	}
}

func deleteTerminal(t *testing.T, f *terminalFixture, session string) int {
	t.Helper()
	req, err := http.NewRequest(
		http.MethodDelete, f.ts.URL+"/api/p/"+f.page+"/terminal/"+session, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

// A shared instance has no terminal surface at all: every route is a plain
// 404, even with a manager passed in and the switch file on in its workdir.
func TestTerminalRoutesAbsentInSharedMode(t *testing.T) {
	f := setupTerminal(t, ModeShared, true)
	if on, err := terminal.Enabled(f.toggle); err != nil || !on {
		t.Fatalf("switch file must be on for this test: %v %v", on, err)
	}
	for _, p := range []string{"/api/terminal", "/api/p/" + f.page + "/terminal/demo"} {
		code, body := get(t, f.ts.URL+p)
		if code != http.StatusNotFound || strings.Contains(body, terminalOffBody) {
			t.Errorf("shared GET %s = %d %q, want a plain 404", p, code, body)
		}
	}
	if code := deleteTerminal(t, f, "demo"); code != http.StatusNotFound {
		t.Errorf("shared DELETE = %d, want 404", code)
	}
	_, resp, err := f.dial(t, "demo", url.Values{"cmd": {"sleep 60"}}, nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("shared socket: err %v, response %v; want 404", err, resp)
	}
	if got := f.mgr.List(); len(got) != 0 {
		t.Errorf("shared mode spawned %+v", got)
	}
}

func TestTerminalSwitchOffKillsAtTheNextCheck(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true)
	r := &terminalReader{conn: f.mustDial(t, "demo", url.Values{"cmd": {"printf up; sleep 60"}})}
	r.outputUntil(t, "up")
	go f.mgr.WatchSwitch(t.Context(), f.toggle, 10*time.Millisecond, t.Logf)

	if err := terminal.Disable(f.toggle); err != nil {
		t.Fatal(err)
	}
	r.until(t, "exit")
	_, _, err := f.dial(t, "demo", nil, nil)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("dial after off: err = %v, want a refused upgrade", err)
	}
}
