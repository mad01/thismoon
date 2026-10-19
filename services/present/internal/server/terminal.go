package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/mad01/thismoon/services/present/internal/terminal"
)

const (
	// terminalOrigin is the d-man route a page opened through the .this
	// front door arrives on. It is load-bearing: d-man rewrites the Host to
	// 127.0.0.1:7423 on the way in, so websocket.Accept's own rule (an
	// Origin equal to the Host passes) never covers that path.
	terminalOrigin = "present.this"
	// terminalOffBody answers every terminal socket while the switch is off.
	terminalOffBody = "live terminal is off: run present terminal on"
	// terminalReadLimit bounds one input frame; a paste arrives as one.
	terminalReadLimit = 1 << 20
	// terminalWriteTimeout drops a browser that stops reading its socket.
	terminalWriteTimeout = 10 * time.Second
	// maxTerminalDim bounds the cols and rows a client may ask for.
	maxTerminalDim = 1000
)

// terminalHello is the first frame of every socket: how many bytes of
// replay follow before the live output.
type terminalHello struct {
	T        string `json:"t"`
	Replayed int    `json:"replayed"`
}

// terminalExit is the last frame once the process has ended.
type terminalExit struct {
	T    string `json:"t"`
	Code int    `json:"code"`
}

// terminalControl is a text frame from the browser: a resize or a kill.
type terminalControl struct {
	T    string `json:"t"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// terminalList is the GET /api/terminal payload.
type terminalList struct {
	Enabled  bool            `json:"enabled"`
	Sessions []terminal.Info `json:"sessions"`
}

// registerTerminal mounts the live terminal routes when a manager is
// configured. Only local mode calls it.
func (s *Server) registerTerminal(mux *http.ServeMux) {
	if s.terminals == nil {
		return
	}
	mux.HandleFunc("GET /api/terminal", s.handleTerminalList)
	mux.HandleFunc("GET /api/p/{id}/terminal/{session}", s.handleTerminal)
	mux.HandleFunc("DELETE /api/p/{id}/terminal/{session}", s.handleTerminalKill)
}

// terminalEnabled reads the switch. It answers on every request, even while
// off, because the browser uses it to choose between the terminal and the
// placeholder naming the enable command.
func (s *Server) terminalEnabled() (bool, error) {
	return terminal.Enabled(s.terminalSwitch)
}

func (s *Server) handleTerminalList(w http.ResponseWriter, _ *http.Request) {
	on, err := s.terminalEnabled()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, terminalList{Enabled: on, Sessions: s.terminals.List()})
}

// handleTerminal upgrades to the WebSocket that carries one session: the
// switch, the session name, the page, the query, and the origin are all
// checked before anything spawns.
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	page, name, ok := s.terminalTarget(w, r)
	if !ok {
		return
	}
	spec, err := parseTerminalSpec(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.terminalOriginAllowed(r) {
		http.Error(w, "terminal: origin not allowed", http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.terminalOrigins,
	})
	if err != nil {
		return // Accept has answered the request.
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(terminalReadLimit)
	sub, err := s.terminals.Attach(page, name, spec)
	if err != nil {
		closeTerminal(conn, err)
		return
	}
	defer sub.Close()
	s.bridgeTerminal(r.Context(), conn, sub, page, name)
}

// terminalTarget checks what the terminal socket names, answering the
// request itself and returning false when it is refused. The switch comes
// first, so an off terminal says nothing about pages or sessions.
func (s *Server) terminalTarget(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	on, err := s.terminalEnabled()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return "", "", false
	}
	if !on {
		http.Error(w, terminalOffBody, http.StatusNotFound)
		return "", "", false
	}
	page, name := r.PathValue("id"), r.PathValue("session")
	if !terminal.ValidName(name) {
		http.Error(w, "terminal: session name must match [a-z0-9-]{1,32}", http.StatusBadRequest)
		return "", "", false
	}
	if _, err := s.store.GetMeta(r.Context(), page); err != nil {
		writeStoreError(w, r, err)
		return "", "", false
	}
	return page, name, true
}

// handleTerminalKill ends one session; 404 when the manager holds none by
// that name. The switch does not gate it: ending a process is always safe.
func (s *Server) handleTerminalKill(w http.ResponseWriter, r *http.Request) {
	if !s.terminalOriginAllowed(r) {
		http.Error(w, "terminal: origin not allowed", http.StatusForbidden)
		return
	}
	err := s.terminals.Kill(r.PathValue("id"), r.PathValue("session"))
	if errors.Is(err, terminal.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// terminalOriginAllowed passes a request with no Origin (the CLI, curl) and
// one whose Origin is loopback or matches an allowed pattern.
// websocket.Accept checks the Origin again, but it always trusts an Origin
// equal to the request's Host, and a DNS name an attacker controls can
// resolve to 127.0.0.1 and make the two agree (DNS rebinding). This check
// never consults the Host. A malformed pattern matches nothing here, and
// Accept logs it.
func (s *Server) terminalOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	for _, pattern := range s.terminalOrigins {
		matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(u.Host))
		if err == nil && matched {
			return true
		}
	}
	return false
}

// parseTerminalSpec reads the spawn parameters from the socket's query.
// A running session ignores them.
func parseTerminalSpec(q url.Values) (terminal.Spec, error) {
	cols, err := terminalDim(q, "cols")
	if err != nil {
		return terminal.Spec{}, err
	}
	rows, err := terminalDim(q, "rows")
	if err != nil {
		return terminal.Spec{}, err
	}
	return terminal.Spec{Cmd: q.Get("cmd"), Cwd: q.Get("cwd"), Cols: cols, Rows: rows}, nil
}

// terminalDim parses one size parameter; absent is zero, the default.
func terminalDim(q url.Values, key string) (uint16, error) {
	v := q.Get(key)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxTerminalDim {
		return 0, errors.New("terminal: " + key + " must be 1 to " + strconv.Itoa(maxTerminalDim))
	}
	return uint16(n), nil
}

// bridgeTerminal runs the socket until either side ends: output goes out on
// its own goroutine, input and control frames come in on this one.
func (s *Server) bridgeTerminal(
	ctx context.Context, conn *websocket.Conn, sub *terminal.Subscription, page, name string,
) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	streamed := make(chan struct{})
	go func() {
		defer close(streamed)
		streamTerminal(ctx, conn, sub)
		// A failed write leaves the input read blocked; closing unblocks
		// it. After a normal close this does nothing.
		_ = conn.CloseNow()
	}()
	s.readTerminalInput(ctx, conn, sub, page, name)
	cancel()
	<-streamed
}

// streamTerminal sends the hello, the replay, and the live output, then the
// exit frame and a normal close once the process ends. A subscriber the
// session dropped for falling behind is told to reconnect, which replays
// what it missed.
func streamTerminal(ctx context.Context, conn *websocket.Conn, sub *terminal.Subscription) {
	replay := sub.Replay()
	if writeTerminalJSON(ctx, conn, terminalHello{T: "hello", Replayed: len(replay)}) != nil {
		return
	}
	if len(replay) > 0 && writeTerminalFrame(ctx, conn, websocket.MessageBinary, replay) != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-sub.Output():
			if !ok {
				endTerminal(ctx, conn, sub)
				return
			}
			if writeTerminalFrame(ctx, conn, websocket.MessageBinary, chunk) != nil {
				return
			}
		}
	}
}

// endTerminal closes the socket once the subscription's output has ended.
func endTerminal(ctx context.Context, conn *websocket.Conn, sub *terminal.Subscription) {
	code, exited := sub.Exit()
	if !exited {
		_ = conn.Close(websocket.StatusTryAgainLater, "terminal: output fell behind, reconnect")
		return
	}
	if writeTerminalJSON(ctx, conn, terminalExit{T: "exit", Code: code}) != nil {
		return
	}
	_ = conn.Close(websocket.StatusNormalClosure, "process exited")
}

// readTerminalInput forwards typed input to the session and applies
// control frames until the socket closes.
func (s *Server) readTerminalInput(
	ctx context.Context, conn *websocket.Conn, sub *terminal.Subscription, page, name string,
) {
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageText {
			s.applyTerminalControl(sub, page, name, data)
			continue
		}
		if _, err := sub.Write(data); err != nil {
			if !errors.Is(err, terminal.ErrExited) {
				log.Printf("present: terminal %s/%s: %v", page, name, err)
			}
			return
		}
	}
}

// applyTerminalControl handles one text frame. A frame it cannot use is
// logged and skipped; the socket stays up.
func (s *Server) applyTerminalControl(
	sub *terminal.Subscription, page, name string, data []byte,
) {
	var c terminalControl
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("present: terminal %s/%s: control frame: %v", page, name, err)
		return
	}
	switch c.T {
	case "resize":
		if err := sub.Resize(c.Cols, c.Rows); err != nil {
			log.Printf("present: terminal %s/%s: %v", page, name, err)
		}
	case "kill":
		// The exit frame follows once the process is gone.
		if err := s.terminals.Kill(page, name); err != nil &&
			!errors.Is(err, terminal.ErrNotFound) {
			log.Printf("present: terminal %s/%s: kill: %v", page, name, err)
		}
	default:
		log.Printf("present: terminal %s/%s: unknown control frame %q", page, name, c.T)
	}
}

// closeTerminal closes a socket whose session could not be attached, with
// the reason the browser can show.
func closeTerminal(conn *websocket.Conn, err error) {
	status := websocket.StatusInternalError
	switch {
	case errors.Is(err, terminal.ErrInvalidSpec):
		status = websocket.StatusPolicyViolation
	case errors.Is(err, terminal.ErrClosed):
		status = websocket.StatusGoingAway
	}
	_ = conn.Close(status, closeReason(err.Error()))
}

// closeReason fits msg into a close frame, which carries at most 123 bytes.
func closeReason(msg string) string {
	const max = 123
	if len(msg) <= max {
		return msg
	}
	return msg[:max]
}

func writeTerminalJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeTerminalFrame(ctx, conn, websocket.MessageText, raw)
}

func writeTerminalFrame(
	ctx context.Context, conn *websocket.Conn, typ websocket.MessageType, p []byte,
) error {
	ctx, cancel := context.WithTimeout(ctx, terminalWriteTimeout)
	defer cancel()
	return conn.Write(ctx, typ, p)
}
