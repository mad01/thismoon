package record

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
)

// finalLine is the transcript line holding stop.json's final message.
const finalLine = `{"type":"assistant","message":{"content":[` +
	`{"type":"text","text":"Renamed the greeting."}]}}` + "\n"

// appendLater appends each part to path after delay, one after another,
// and returns a channel closed once the last is written.
func appendLater(t *testing.T, path string, delay time.Duration, parts ...string) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, part := range parts {
			time.Sleep(delay)
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				t.Error(err)
				return
			}
			_, err = f.WriteString(part)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				t.Error(err)
				return
			}
		}
	}()
	return done
}

// TestStopWaitsForTheFinalMessage: Claude Code writes the transcript
// asynchronously, so the Stop hook can run before the turn's last response
// is in the file. The close waits until the transcript holds the payload's
// final message, here written in two pieces after the hook started, and
// keeps the turn's lines through it.
func TestStopWaitsForTheFinalMessage(t *testing.T) {
	s := newSession(t, physicalTempDir(t))
	s.flushWait = 10 * time.Second
	before := "{\"type\":\"user\",\"line\":\"before\"}\n"
	writeFile(t, s.transcript, before)
	s.mustRun("prompt.json")
	s.take()

	working := "{\"type\":\"user\",\"line\":1}\n"
	writeFile(t, s.transcript, before+working)
	half := len(finalLine) / 2
	done := appendLater(t, s.transcript, 50*time.Millisecond, finalLine[:half], finalLine[half:])
	s.mustRun("stop.json")
	<-done

	tc := s.takeOne(spool.KindTurnClose).TurnClose
	if got := s.chunk(tc.TranscriptChunk); got != working+finalLine {
		t.Errorf("transcript = %q, want the lines through the final message %q", got, working+finalLine)
	}
	if want := int64(len(before + working + finalLine)); tc.TranscriptEnd != want {
		t.Errorf("TranscriptEnd = %d, want %d", tc.TranscriptEnd, want)
	}
}

// TestStopFlushWaitRunsOut: a final message that never reaches the
// transcript costs the flush wait and no more. The close records the lines
// there are, and the head-moved report after it still gets its own budget.
func TestStopFlushWaitRunsOut(t *testing.T) {
	repo := newRepo(t)
	s := newSession(t, repo)
	writeFile(t, s.transcript, "{\"type\":\"user\",\"line\":\"before\"}\n")
	writeFile(t, filepath.Join(repo, "a.txt"), "one\ndeux\n")
	s.mustRun("edit.json", "$FILE", filepath.Join(repo, "a.txt"), "$TOOL_USE_ID", "toolu_edit_1")
	s.mustRun("prompt.json")
	s.take()
	git(t, repo, "commit", "-q", "-am", "work")

	working := "{\"type\":\"user\",\"line\":1}\n"
	writeFile(t, s.transcript, "{\"type\":\"user\",\"line\":\"before\"}\n"+working)
	s.budget, s.flushWait = 500*time.Millisecond, time.Second
	start := time.Now()
	s.mustRun("stop.json")
	took := time.Since(start)

	envs := s.take()
	want := []spool.Kind{spool.KindTurnClose, spool.KindHeadMoved, spool.KindSnapshot}
	if !slices.Equal(kinds(envs), want) {
		t.Fatalf("stop wrote %v, want %v", kinds(envs), want)
	}
	if got := s.chunk(envs[0].TurnClose.TranscriptChunk); got != working {
		t.Errorf("transcript = %q, want the lines there were %q", got, working)
	}
	if took < s.flushWait {
		t.Errorf("stop took %s, want at least the flush wait %s", took, s.flushWait)
	}
}

// TestStopWithoutFinalDoesNotWait: a Stop payload with no final message
// has nothing to wait for.
func TestStopWithoutFinalDoesNotWait(t *testing.T) {
	s := newSession(t, physicalTempDir(t))
	s.flushWait = 10 * time.Second
	writeFile(t, s.transcript, "{\"line\":\"before\"}\n")
	s.mustRun("prompt.json")
	s.take()

	start := time.Now()
	if err := s.runRaw(`{"session_id":"sess-1","prompt_id":"prompt-1","hook_event_name":"Stop",` +
		`"transcript_path":"` + s.transcript + `","cwd":"` + s.cwd + `"}`); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took >= s.flushWait {
		t.Errorf("stop without a final message took %s, want no wait", took)
	}
	s.takeOne(spool.KindTurnClose)
}

// TestTextValue: the needle is the final text as JSON.stringify writes a
// content block's value, with no HTML escapes and the line separators raw.
func TestTextValue(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"plain", "Done.", `"text":"Done."`},
		{"quotes and controls", "a \"b\"\n\tc\\", `"text":"a \"b\"\n\tc\\"`},
		{"html stays raw", "<a href=x>&</a>", `"text":"<a href=x>&</a>"`},
		{"line separators stay raw", "one\u2028two\u2029", "\"text\":\"one\u2028two\u2029\""},
		{"unicode", "ok ✓ 🚀", `"text":"ok ✓ 🚀"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(textValue(tc.text)); got != tc.want {
				t.Errorf("textValue(%q) = %s, want %s", tc.text, got, tc.want)
			}
		})
	}
}
