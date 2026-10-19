package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
	"github.com/mad01/thismoon/services/present/internal/terminal"
)

// terminalDeadline bounds every socket exchange; generous because a
// process launch on a loaded machine can take seconds.
const terminalDeadline = 30 * time.Second

type terminalFixture struct {
	ts     *httptest.Server
	st     store.Store
	mgr    *terminal.Manager
	page   string
	toggle string
}

// termBlock is a terminal block for session that runs cmd.
func termBlock(session, cmd string) render.Block {
	return render.Block{T: "terminal", Session: session, Cmd: cmd}
}

// setupTerminal serves one page whose brief holds blocks, with a terminal
// manager whose sessions start in a temp home and the switch on or off.
func setupTerminal(t *testing.T, mode Mode, on bool, blocks ...render.Block) *terminalFixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatalf("store.NewFS: %v", err)
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
	f := &terminalFixture{ts: ts, st: st, mgr: mgr, toggle: toggle}
	f.page = f.addPage(t, blocks, nil)
	return f
}

// addPage stores a page whose brief holds brief and, when deck is not nil,
// whose deck holds deck, each compiled the way the MCP tools compile them.
func (f *terminalFixture) addPage(t *testing.T, brief, deck []render.Block) string {
	t.Helper()
	if len(brief) == 0 {
		brief = []render.Block{{T: "p", Text: "x"}}
	}
	compiled, err := render.CompileDoc(terminalDoc(brief), "T")
	if err != nil {
		t.Fatalf("compile brief: %v", err)
	}
	d := store.Draft{Title: "T", Content: compiled.HTML, Doc: compiled.JSON}
	if deck != nil {
		c, err := render.CompileDeckDoc(terminalDoc(deck), "T")
		if err != nil {
			t.Fatalf("compile deck: %v", err)
		}
		d.Deck, d.DeckSource = c.HTML, c.JSON
	}
	p, err := f.st.Create(t.Context(), d)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return p.ID
}

func terminalDoc(blocks []render.Block) render.Doc {
	return render.Doc{Sections: []render.Section{{Heading: "S", Blocks: blocks}}}
}

func (f *terminalFixture) socketURL(page, session string, q url.Values) string {
	u := "ws" + strings.TrimPrefix(f.ts.URL, "http") + "/api/p/" + page + "/terminal/" + session
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (f *terminalFixture) dial(
	t *testing.T, session string, q url.Values, opts *websocket.DialOptions,
) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	return f.dialPage(t, f.page, session, q, opts)
}

// dialPage opens a terminal socket for session on page. The read limit is
// lifted, since a frame carries every byte of output queued for the socket.
func (f *terminalFixture) dialPage(
	t *testing.T, page, session string, q url.Values, opts *websocket.DialOptions,
) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), terminalDeadline)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, f.socketURL(page, session, q), opts)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if conn != nil {
		conn.SetReadLimit(64 << 20)
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
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", "sleep 60"))
	cases := []struct {
		path string
		want int
		body string
	}{
		{"/api/p/" + f.page + "/terminal/Bad_Name", http.StatusBadRequest, ""},
		{"/api/p/" + f.page + "/terminal/" + strings.Repeat("a", 33), http.StatusBadRequest, ""},
		{"/api/p/missing0000/terminal/demo", http.StatusNotFound, ""},
		{"/api/p/" + f.page + "/terminal/demo?cols=0", http.StatusBadRequest, ""},
		{"/api/p/" + f.page + "/terminal/demo?rows=wide", http.StatusBadRequest, ""},
		{
			"/api/p/" + f.page + "/terminal/undeclared?cmd=sleep+60", http.StatusNotFound,
			`page declares no terminal session "undeclared"`,
		},
	}
	for _, tc := range cases {
		code, body := get(t, f.ts.URL+tc.path)
		if code != tc.want || !strings.Contains(body, tc.body) {
			t.Errorf("GET %s = %d %q, want %d %q", tc.path, code, body, tc.want, tc.body)
		}
	}
	if got := f.mgr.List(); len(got) != 0 {
		t.Errorf("refused requests spawned %+v", got)
	}
}

func TestTerminalRefusesForeignOrigins(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", "sleep 60"))
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
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, resp, err := f.dial(t, "demo", nil, tc.opts)
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
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", "printf hi; cat"))
	loopback := strings.TrimPrefix(f.ts.URL, "http://")
	if !strings.HasPrefix(loopback, "127.0.0.1:") {
		t.Fatalf("test server on %s, want a 127.0.0.1 address", loopback)
	}
	dialFrom := func(origin string) (*websocket.Conn, *http.Response, error) {
		return f.dial(t, "demo", nil, &websocket.DialOptions{
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
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", "printf hi; exit 4"))
	r := &terminalReader{conn: f.mustDial(t, "demo", nil)}

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
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", "printf ready; cat"))
	first := &terminalReader{conn: f.mustDial(t, "demo", nil)}
	first.outputUntil(t, "ready")
	_ = first.conn.Close(websocket.StatusNormalClosure, "tab reloaded")

	again := &terminalReader{conn: f.mustDial(t, "demo", nil)}
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
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", "printf up; sleep 60"))
	r := &terminalReader{conn: f.mustDial(t, "demo", nil)}
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
	f := setupTerminal(t, ModeShared, true, termBlock("demo", "sleep 60"))
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
	_, resp, err := f.dial(t, "demo", nil, nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("shared socket: err %v, response %v; want 404", err, resp)
	}
	if got := f.mgr.List(); len(got) != 0 {
		t.Errorf("shared mode spawned %+v", got)
	}
}

func TestTerminalSwitchOffKillsAtTheNextCheck(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", "printf up; sleep 60"))
	r := &terminalReader{conn: f.mustDial(t, "demo", nil)}
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

// What a session runs comes from the stored page alone: a socket that
// names a cmd and a cwd in its query still gets the block's command.
func TestTerminalRunsTheStoredSpecNotTheQuery(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", "printf stored; exit 0"))
	q := url.Values{"cmd": {"printf injected; exit 9"}, "cwd": {"/"}}
	r := &terminalReader{conn: f.mustDial(t, "demo", q)}

	exit := r.until(t, "exit")
	if exit["code"] != float64(0) || !strings.Contains(r.output.String(), "stored") ||
		strings.Contains(r.output.String(), "injected") {
		t.Errorf("exit %v, output %q; want the stored command's", exit, r.output.String())
	}
	if got := f.mgr.List(); len(got) != 1 || got[0].Cmd != "printf stored; exit 0" {
		t.Errorf("List() = %+v, want the stored command", got)
	}
}

// A deck is a rendition of the same page, so a session its blocks declare
// attaches like one the brief declares.
func TestTerminalFindsASessionTheDeckDeclares(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true)
	page := f.addPage(t, nil, []render.Block{termBlock("slides", "printf from-deck; exit 0")})
	conn, _, err := f.dialPage(t, page, "slides", nil, nil)
	if err != nil {
		t.Fatalf("dial the deck's session: %v", err)
	}
	r := &terminalReader{conn: conn}
	r.until(t, "exit")
	if !strings.Contains(r.output.String(), "from-deck") {
		t.Errorf("output = %q, want the deck block's command", r.output.String())
	}
	if code, _ := get(t, f.ts.URL+"/api/p/"+f.page+"/terminal/slides"); code != http.StatusNotFound {
		t.Errorf("another page's session = %d, want 404", code)
	}
}

// The prefill comes from the stored block too, typed without a newline.
func TestTerminalTypesTheStoredPrefill(t *testing.T) {
	block := termBlock("demo", "printf 'name? '; read x; echo got:$x")
	block.Prefill = "stored"
	f := setupTerminal(t, ModeLocal, true, block)
	r := &terminalReader{conn: f.mustDial(t, "demo", url.Values{"prefill": {"injected"}})}

	r.outputUntil(t, "stored")
	if err := r.conn.Write(t.Context(), websocket.MessageBinary, []byte("\r")); err != nil {
		t.Fatalf("write Enter: %v", err)
	}
	r.outputUntil(t, "got:stored")
	if strings.Contains(r.output.String(), "injected") {
		t.Errorf("output = %q, want only the stored prefill", r.output.String())
	}
}

// A program printing a byte at a time must not drop a client that keeps
// up: the session counts what it is behind in bytes, and each frame
// carries everything queued since the last.
func TestTerminalByteAtATimeOutputReachesTheClient(t *testing.T) {
	const total = 300000
	cmd := "for i in {1..300000}; do printf x; done; exit 3"
	f := setupTerminal(t, ModeLocal, true, termBlock("drip", cmd))
	r := &terminalReader{conn: f.mustDial(t, "drip", nil)}

	if exit := r.until(t, "exit"); exit["code"] != float64(3) {
		t.Errorf("exit = %v, want code 3", exit)
	}
	if got := strings.Count(r.output.String(), "x"); got != total {
		t.Errorf("client read %d x, want %d", got, total)
	}
	if st := r.closeStatus(t); st != websocket.StatusNormalClosure {
		t.Errorf("close status = %v, want normal closure", st)
	}
}

// Input the process is not reading must not hold up the socket: a kill
// sent after it still ends the session.
func TestTerminalKillGetsPastStuckInput(t *testing.T) {
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", "stty raw -echo; printf up; sleep 60"))
	r := &terminalReader{conn: f.mustDial(t, "demo", nil)}
	r.outputUntil(t, "up")

	paste := bytes.Repeat([]byte("x"), 512<<10)
	for range 2 {
		if err := r.conn.Write(t.Context(), websocket.MessageBinary, paste); err != nil {
			t.Fatalf("write input: %v", err)
		}
	}
	writeText(t, r.conn, map[string]string{"t": "kill"})
	if exit := r.until(t, "exit"); exit["code"] == float64(0) {
		t.Errorf("exit after kill = %v, want a signal code", exit)
	}
}

// Input arriving after the process ended is dropped while the socket goes
// on, so the output still on its way and the exit frame are delivered.
func TestTerminalInputAfterTheExitKeepsTheTailAndTheExit(t *testing.T) {
	const total = 3_000_000
	cmd := fmt.Sprintf("head -c %d /dev/zero | tr '\\0' a; exit 7", total)
	f := setupTerminal(t, ModeLocal, true, termBlock("demo", cmd))
	conn := f.mustDial(t, "demo", nil)
	waitForTerminal(t, "the process to exit", func() bool {
		list := f.mgr.List()
		return len(list) == 1 && list[0].Exited
	})

	if err := conn.Write(t.Context(), websocket.MessageBinary, []byte("x")); err != nil {
		t.Fatalf("write input: %v", err)
	}
	r := &terminalReader{conn: conn}
	if exit := r.until(t, "exit"); exit["code"] != float64(7) {
		t.Errorf("exit = %v, want code 7", exit)
	}
	if got := strings.Count(r.output.String(), "a"); got != total {
		t.Errorf("read %d bytes of output, want %d", got, total)
	}
}

func waitForTerminal(t *testing.T, what string, cond func() bool) {
	t.Helper()
	end := time.Now().Add(terminalDeadline)
	for !cond() {
		if time.Now().After(end) {
			t.Fatalf("%s: not within %s", what, terminalDeadline)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
