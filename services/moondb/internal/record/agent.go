package record

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/services/moondb/internal/agentfile"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// closeAgent records a subagent as it stopped: its agent transcript, read
// from agentTranscriptFile's path and cut to heads (agentfile.Read), with the
// meta file beside it and the git place of the agent's cwd, in one
// agent-close envelope. The envelope is named by the agent and the lines
// read, so a replayed payload, or a second stop with nothing new in the
// file, writes the same envelope again. A transcript that cannot be read
// still records the agent, without lines, as the meta file and the
// payload describe it, unless that is a main segment's record
// (mainSegment). That, a path outside the session's subagents directory,
// or a session_id that is not one path element records nothing and is
// logged.
func (rec *recorder) closeAgent() error {
	p := rec.p
	path, err := agentTranscriptFile(p)
	if err != nil {
		rec.opts.Logf("agent %s: %v; not recorded", p.agentID, err)
		return nil
	}
	c, readErr := agentfile.Read(rec.ctx, path, p.agentType)
	if readErr != nil {
		c = agentfile.Capture{}
		c.Type, c.Name = agentfile.Kind(agentfile.ReadMeta(agentfile.MetaPath(path)), p.agentType)
	}
	chunks := chunkSet{}
	ac := rec.agentClose(c, chunks)
	switch {
	case mainSegment(ac):
		rec.opts.Logf("agent %s: %v; an %s agent with no line read, "+
			"a main segment or one with nothing to keep, is not recorded",
			p.agentID, readErr, store.InternalAgent)
		return nil
	case readErr != nil:
		rec.opts.Logf("agent %s: %v; recording it without lines", p.agentID, readErr)
	}
	env := rec.envelope(spool.KindAgentClose, fmt.Sprintf("%s:close:%d", p.agentID, c.Lines.Read))
	env.AgentClose = ac
	return rec.spool.Write(env, chunks)
}

// agentClose is the agent-close payload of c, its agent transcript and its
// brief added to chunks. It ends when the hook ran, and starts then too
// when no line says when it started.
func (rec *recorder) agentClose(c agentfile.Capture, chunks chunkSet) *spool.AgentClose {
	ac := &spool.AgentClose{
		Type: c.Type, Name: c.Name, Description: c.Description, Model: c.Model,
		TurnIDs: c.TurnIDs, Cwd: c.Cwd, StartedAt: c.StartedAt, EndedAt: rec.start,
		Lines: c.Lines,
	}
	if ac.StartedAt.IsZero() {
		ac.StartedAt = rec.start
	}
	if len(c.Transcript) > 0 {
		ac.TranscriptChunk = chunks.add(c.Transcript)
	}
	if c.Brief != "" {
		ac.BriefChunk = chunks.add([]byte(c.Brief))
	}
	ac.LaterMessages = c.LaterMessages
	if c.Cwd != "" {
		ac.Location = rec.locate(c.Cwd)
	}
	return ac
}

// mainSegment reports whether ac would make a main segment's record
// (store.Agent.IsMainSegment), which ingest drops and no listing shows: an
// internal agent with no line read. A main segment's stop gives one, since
// the transcript file it names never exists, and so does an untyped agent
// whose file holds no timeline line. A later import keeps that agent once
// its file has lines.
func mainSegment(ac *spool.AgentClose) bool {
	a := store.Agent{Type: ac.Type, Lines: ac.Lines, TranscriptChunk: ac.TranscriptChunk}
	return a.IsMainSegment()
}

// agentTranscriptFile is the file closeAgent reads: agent_transcript_path with
// a leading ~ expanded, or, when the payload names none (a harness from
// before the field), the path the harness keeps it at:
// <dir of transcript_path>/<session_id>/subagents/agent-<agent_id>.jsonl.
// A path outside that subagents directory, symlinks resolved, is an error,
// and so is a session_id that is not one path element: it would move the
// directory itself.
func agentTranscriptFile(p payload) (string, error) {
	main, err := confdir.Expand(p.transcriptPath)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(main) {
		return "", errors.New("no absolute transcript_path to find the agent transcript by")
	}
	if !pathElement(p.sessionID) {
		return "", fmt.Errorf("session_id %q is not a single path element", p.sessionID)
	}
	dir := filepath.Join(filepath.Dir(main), p.sessionID, "subagents")
	path := filepath.Join(dir, "agent-"+p.agentID+".jsonl")
	if p.agentTranscriptPath != "" {
		if path, err = confdir.Expand(p.agentTranscriptPath); err != nil {
			return "", err
		}
	}
	if !within(dir, path) {
		return "", fmt.Errorf("agent transcript %s is outside %s", path, dir)
	}
	return path, nil
}

// within reports whether path lies under dir. When both exist their
// symlinks are resolved first, so a link cannot lead out of dir.
func within(dir, path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		if realDir, err := filepath.EvalSymlinks(dir); err == nil {
			dir, path = realDir, real
		}
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// pathElement reports whether name is one path element: not empty, not
// "." or "..", and without a separator.
func pathElement(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!strings.ContainsRune(name, filepath.Separator)
}
