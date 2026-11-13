package guard

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/cslindex"
)

// LargeReadID identifies the guard against whole-file reads of large files.
const LargeReadID = "large-read"

// nonTextExts are Read targets that are not lines of text. Read renders an
// image as pixels, a PDF by pages and a notebook as cells, so a line count
// says nothing about what the read costs.
var nonTextExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".ico": true, ".tif": true, ".tiff": true, ".heic": true,
	".pdf": true, ".ipynb": true,
}

// editTools are the tool calls that change the file their input's file_path
// names. NotebookEdit names a notebook_path instead, and a notebook read
// passes anyway.
var editTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true}

// toolUseMarker picks the transcript lines worth decoding: a line holding a
// tool call carries it.
var toolUseMarker = []byte(`"tool_use"`)

// lineCountChunk is how much of a file one read takes while counting lines.
const lineCountChunk = 32 << 10

// LargeRead refuses a Read with no offset and no limit of a text file of
// large_file_lines or more that csl indexes, in a csl-indexed repo or a
// linked worktree of one, and hands over the csl_ask call for that file
// instead, or for a file too long for any csl_ask call says to read it in
// ranges. The read-size hint said the same after the read, when the tokens
// were already spent; this says it before. A ranged read always passes, and
// so does a file that call cannot answer from and a file this agent already
// edited or wrote in the session. Everything the guard cannot judge passes:
// a wrong refusal blocks work, a missed one costs tokens.
type LargeRead struct {
	minLines int
}

// NewLargeRead builds the guard. Besides enabled, it reads only the
// top-level large_file_lines, the threshold it shares with the read-size
// hint.
func NewLargeRead(cfg config.Config) *LargeRead {
	return &LargeRead{minLines: cfg.LargeFileThreshold()}
}

func (g *LargeRead) ID() string    { return LargeReadID }
func (g *LargeRead) Event() string { return EventRead }

// Check runs the cheap tests first: the range and the path need no
// filesystem, and the line count stops at the threshold. Only after the
// line count and the index lookup pass is the whole file read for
// csl_ask's NUL check and line count, and the transcript only after that.
func (g *LargeRead) Check(in Input) *Denial {
	if in.Ranged || !filepath.IsAbs(in.FilePath) {
		return nil
	}
	file := filepath.Clean(in.FilePath)
	if nonTextExts[strings.ToLower(filepath.Ext(file))] || !hasLines(file, g.minLines) {
		return nil
	}
	loc, ok := cslindex.Resolve(file)
	if !ok {
		return nil
	}
	lines, ok := cslindex.AskLines(loc.Root, loc.Rel)
	if !ok {
		return nil
	}
	if edited, known := editedByAgent(in.TranscriptPath, in.AgentID, file); edited || !known {
		return nil
	}
	return g.refuse(file, loc, lines)
}

// refuse is the three-line refusal: what the read would cost, then the way
// past it. A file csl_ask can read gets the call for it; one past
// csl_ask's line ceiling gets the ranged read alone, since no call can
// read it.
func (g *LargeRead) refuse(file string, loc cslindex.Located, lines int) *Denial {
	const ranged = "a ranged read passes this guard."
	call, ok := loc.AskCall(lines)
	if !ok {
		return Reasonf(
			LargeReadID,
			"%s has %d lines, and a Read without offset and limit puts all of them in context.\n"+
				"%s.\n"+
				"Read it in ranges with offset and limit instead: "+ranged,
			file, lines, cslindex.AskTooLong,
		)
	}
	return Reasonf(
		LargeReadID,
		"%s has %d lines or more, and a Read without offset and limit puts all of them in context.\n"+
			"When you need an answer from it, ask instead: %s returns the answer, not the file.\n"+
			"When you will edit it, Read the region with offset and limit: "+ranged,
		file,
		g.minLines,
		call,
	)
}

// hasLines reports whether file is a readable text file of at least n
// lines, the last one counted with or without a trailing newline. It stops
// reading once the count reaches n. A missing file, a directory, a file it
// cannot read and a file with a NUL byte (binary) all report false: Read
// handles those itself, and the guard has nothing to say about them.
func hasLines(file string, n int) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return false
	}
	buf := make([]byte, lineCountChunk)
	lines, open := 0, false // open: the last line read has no newline yet
	for {
		k, err := f.Read(buf)
		chunk := buf[:k]
		if bytes.IndexByte(chunk, 0) >= 0 {
			return false
		}
		lines += bytes.Count(chunk, []byte{'\n'})
		if lines >= n {
			return true
		}
		if k > 0 {
			open = chunk[k-1] != '\n'
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false
		}
	}
	if open {
		lines++
	}
	return lines >= n
}

// editedByAgent reports whether the agent making this call already edited
// or wrote file, from the transcript Claude Code keeps for it. The main
// thread's tool calls are in the session transcript the payload names. A
// subagent's are in a file of its own, agent-<id>.jsonl under
// <session>/subagents/ beside the session transcript, and only those count
// for it: a subagent reviewing what the main thread changed is reading, not
// editing. known is false when that transcript cannot be found or read, and
// the guard then allows.
func editedByAgent(transcriptPath, agentID, file string) (edited, known bool) {
	path, ok := agentTranscript(transcriptPath, agentID)
	if !ok {
		return false, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	return transcriptEdits(raw, file), true
}

// agentTranscript names the transcript holding the calling agent's own tool
// calls. Inside a subagent the payload's transcript_path may name the
// session transcript or the subagent's own file; both resolve to the
// latter. ok is false without a transcript path, and for an agent id that
// is not a plain file name part.
func agentTranscript(transcriptPath, agentID string) (string, bool) {
	if transcriptPath == "" {
		return "", false
	}
	if agentID == "" {
		return transcriptPath, true
	}
	own := "agent-" + agentID + ".jsonl"
	if filepath.Base(own) != own || strings.Contains(agentID, "..") {
		return "", false
	}
	if filepath.Base(transcriptPath) == own {
		return transcriptPath, true
	}
	session := strings.TrimSuffix(transcriptPath, filepath.Ext(transcriptPath))
	return filepath.Join(session, "subagents", own), true
}

// transcriptEdits reports whether a transcript holds an Edit, Write or
// MultiEdit call on file. A line is decoded only when it carries a tool
// call and the file's base name, so a long transcript costs one scan.
func transcriptEdits(raw []byte, file string) bool {
	base := []byte(filepath.Base(file))
	for line := range bytes.Lines(raw) {
		if !bytes.Contains(line, base) || !bytes.Contains(line, toolUseMarker) {
			continue
		}
		var rec struct {
			Message struct {
				Content []struct {
					Type  string `json:"type"`
					Name  string `json:"name"`
					Input struct {
						FilePath string `json:"file_path"`
					} `json:"input"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		for _, c := range rec.Message.Content {
			if c.Type == "tool_use" && editTools[c.Name] &&
				filepath.IsAbs(c.Input.FilePath) && filepath.Clean(c.Input.FilePath) == file {
				return true
			}
		}
	}
	return false
}
