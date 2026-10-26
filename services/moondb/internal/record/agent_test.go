package record

import (
	"context"
	"os"
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

// agentFile is where the harness keeps agent-7's transcript in s: under
// sess-1/subagents beside the main transcript.
func agentFile(s *testSession) string {
	return filepath.Join(filepath.Dir(s.transcript), "sess-1", "subagents", "agent-agent-7.jsonl")
}

// TestSubagentStop records the agent's transcript, cut to heads, with who
// it was and where it worked, named by the lines read.
func TestSubagentStop(t *testing.T) {
	repo := newRepo(t)
	s := newSession(t, repo)
	file := agentFile(s)
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
	s.mustRun("subagent_stop.json", "$AGENT_TRANSCRIPT", agentFile(s))
	env := s.takeOne(spool.KindAgentClose)
	ac := env.AgentClose
	if env.ID != "agent-7:close:0" || ac.Type != "worker" || ac.TranscriptChunk != "" ||
		!ac.StartedAt.Equal(testNow) || len(s.logs) != 1 {
		t.Errorf("envelope %s = %+v, logs %q; want the worker without lines", env.ID, ac, s.logs)
	}
}

// TestSubagentStopTranscriptPath reads the agent transcript where the
// payload says, a leading ~ expanded, or where the harness keeps it when
// the payload names none, and records nothing for a path outside the
// session's subagents directory.
func TestSubagentStopTranscriptPath(t *testing.T) {
	outside := filepath.Join(physicalTempDir(t), "agent-agent-7.jsonl")
	writeFile(t, outside, agentTranscript("/work"))
	cases := []struct {
		name string
		path func(s *testSession) string
		want string // the envelope id; "" for none
	}{
		{"named", agentFile, "agent-7:close:3"},
		{"empty field", func(*testSession) string { return "" }, "agent-7:close:3"},
		{"tilde", func(s *testSession) string {
			s.tb.Setenv("HOME", filepath.Dir(s.transcript))
			return "~/sess-1/subagents/agent-agent-7.jsonl"
		}, "agent-7:close:3"},
		{"outside", func(*testSession) string { return outside }, ""},
		{"climbs out", func(s *testSession) string {
			return filepath.Join(filepath.Dir(agentFile(s)), "..", "..", "x.jsonl")
		}, ""},
		{"symlink out", func(s *testSession) string {
			link := filepath.Join(filepath.Dir(agentFile(s)), "agent-link.jsonl")
			if err := os.Symlink(outside, link); err != nil {
				s.tb.Fatal(err)
			}
			return link
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSession(t, physicalTempDir(t))
			writeFile(t, agentFile(s), agentTranscript(s.cwd))
			s.mustRun("subagent_stop.json", "$AGENT_TRANSCRIPT", c.path(s))
			envs := s.take()
			switch {
			case c.want == "" && (len(envs) != 0 || len(s.logs) != 1):
				t.Errorf("spooled %v, logs %q; want nothing and one log line", kinds(envs), s.logs)
			case c.want != "" && (len(envs) != 1 || envs[0].ID != c.want):
				t.Errorf("spooled %v, logs %q; want %s", kinds(envs), s.logs, c.want)
			}
		})
	}
}
