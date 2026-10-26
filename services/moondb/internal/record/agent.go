package record

import (
	"fmt"

	"github.com/mad01/thismoon/services/moondb/internal/agentfile"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
)

// closeAgent records a subagent as it stopped: its agent transcript, read
// from agent_transcript_path and cut to heads (agentfile.Read), with the
// meta file beside it and the git place of the agent's cwd, in one
// agent-close envelope. The envelope is named by the agent and the lines
// read, so a replayed payload, or a second stop with nothing new in the
// file, writes the same envelope again. A transcript that cannot be read
// still records the agent, without lines, as the meta file and the
// payload describe it.
func (rec *recorder) closeAgent() error {
	p := rec.p
	c, err := agentfile.Read(rec.ctx, p.agentTranscriptPath, p.agentType)
	if err != nil {
		rec.opts.Logf("agent %s: %v; recording it without lines", p.agentID, err)
		c = agentfile.Capture{}
		c.Type, c.Name = agentfile.Kind(agentfile.ReadMeta(agentfile.MetaPath(
			p.agentTranscriptPath)), p.agentType)
	}
	env := rec.envelope(spool.KindAgentClose, fmt.Sprintf("%s:close:%d", p.agentID, c.Lines.Read))
	chunks := chunkSet{}
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
	if c.Cwd != "" {
		ac.Location = rec.locate(c.Cwd)
	}
	env.AgentClose = ac
	return rec.spool.Write(env, chunks)
}
