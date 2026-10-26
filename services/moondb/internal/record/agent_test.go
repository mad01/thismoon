package record

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/agentfile"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
)

// agentTranscript is a subagent's transcript working in dir: messaged
// under prompt-1, one model call that reads a file.
func agentTranscript(dir string) string {
	return `{"type":"user","isSidechain":true,"agentId":"agent-7","promptId":"prompt-1",` +
		`"cwd":"` + dir + `","timestamp":"2026-10-25T11:58:00Z","message":{"content":"go"}}
{"type":"attachment","isSidechain":true,"agentId":"agent-7","timestamp":"2026-10-25T11:58:00Z"}
{"type":"assistant","isSidechain":true,"agentId":"agent-7","cwd":"` + dir + `",` +
		`"timestamp":"2026-10-25T11:59:00Z","message":{"id":"m1","model":"claude-opus-5-5",` +
		`"content":[{"type":"text","text":"done"}],"usage":{"input_tokens":5,"output_tokens":1}}}
`
}

// TestSubagentStop records the agent's transcript, cut to heads, with who
// it was and where it worked, named by the lines read.
func TestSubagentStop(t *testing.T) {
	repo := newRepo(t)
	s := newSession(t, repo)
	file := filepath.Join(physicalTempDir(t), "agent-agent-7.jsonl")
	writeFile(t, file, agentTranscript(repo))
	writeFile(t, agentfile.MetaPath(file),
		`{"agentType":"builder","name":"builder","spawnedAgentType":"general-purpose",`+
			`"description":"Build it"}`)
	s.mustRun("subagent_stop.json", "$AGENT_TRANSCRIPT", file)

	env := s.takeOne(spool.KindAgentClose)
	ac := env.AgentClose
	want, err := agentfile.Read(context.Background(), file, "worker")
	if err != nil {
		t.Fatal(err)
	}
	checkFields(t, []field{
		{"ID", env.ID, "agent-7:close:3"},
		{"Agent.ID", env.Agent.ID, "agent-7"},
		{"Type", ac.Type, "general-purpose"},
		{"Name", ac.Name, "builder"},
		{"Description", ac.Description, "Build it"},
		{"Model", ac.Model, "claude-opus-5-5"},
		{"Cwd", ac.Cwd, repo},
		{"Location.Repo", ac.Location.Repo, "github.com/example/demo"},
		{"transcript", s.chunk(ac.TranscriptChunk), string(want.Transcript)},
	})
	if !slices.Equal(ac.TurnIDs, []string{"prompt-1"}) || ac.Lines.Read != 3 ||
		ac.Lines.Kept != 2 || !ac.EndedAt.Equal(testNow) ||
		!ac.StartedAt.Equal(time.Date(2026, 10, 25, 11, 58, 0, 0, time.UTC)) {
		t.Errorf("AgentClose = %+v", ac)
	}
}

// TestSubagentStopWithoutTranscript still records the agent, as the
// payload names it, starting and ending when the hook ran.
func TestSubagentStopWithoutTranscript(t *testing.T) {
	s := newSession(t, physicalTempDir(t))
	s.mustRun("subagent_stop.json", "$AGENT_TRANSCRIPT", filepath.Join(s.cwd, "missing.jsonl"))
	env := s.takeOne(spool.KindAgentClose)
	ac := env.AgentClose
	if env.ID != "agent-7:close:0" || ac.Type != "worker" || ac.TranscriptChunk != "" ||
		!ac.StartedAt.Equal(testNow) || len(s.logs) != 1 {
		t.Errorf("envelope %s = %+v, logs %q; want the worker without lines", env.ID, ac, s.logs)
	}
}
