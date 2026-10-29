package record

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/agentfile"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
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
// it was, the brief it was spawned with and where it worked, named by the
// lines read.
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
		{"brief", s.chunk(ac.BriefChunk), "go"},
	})
	if !slices.Equal(ac.TurnIDs, []string{"prompt-1"}) || ac.Lines.Read != 3 ||
		ac.Lines.Kept != 2 || !ac.EndedAt.Equal(testNow) ||
		!ac.StartedAt.Equal(time.Date(2026, 10, 25, 11, 58, 0, 0, time.UTC)) {
		t.Errorf("AgentClose = %+v", ac)
	}
}

// TestSubagentStopWithoutTranscript still records an agent whose
// transcript cannot be read, as the payload and the meta file name it,
// starting and ending when the hook ran, unless that leaves an internal
// agent with no line read: a main segment's record, which ingest drops.
// That records nothing. Each case logs one line.
func TestSubagentStopWithoutTranscript(t *testing.T) {
	cases := []struct {
		name    string
		untyped bool // the payload names no agent_type
		setup   func(t *testing.T, file string)
		want    string // the recorded type; "" for nothing recorded
		log     string
	}{
		{"typed", false, nil, "worker", "recording it without lines"},
		{"untyped", true, nil, "", "no line read"},
		{"untyped with a meta file", true, func(t *testing.T, file string) {
			writeFile(t, agentfile.MetaPath(file), `{"spawnedAgentType":"general-purpose"}`)
		}, "general-purpose", "recording it without lines"},
		{"untyped with an empty transcript", true, func(t *testing.T, file string) {
			writeFile(t, file, "")
		}, "", "no line read"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSession(t, physicalTempDir(t))
			file := agentFile(s)
			if c.setup != nil {
				c.setup(t, file)
			}
			vars := []string{"$AGENT_TRANSCRIPT", file}
			if c.untyped {
				vars = append(vars, `"agent_type": "worker",`, "")
			}
			s.mustRun("subagent_stop.json", vars...)
			envs := s.take()
			if len(s.logs) != 1 || !strings.Contains(s.logs[0], c.log) {
				t.Errorf("logs %q, want one saying %q", s.logs, c.log)
			}
			if c.want == "" {
				if len(envs) != 0 {
					t.Errorf("spooled %v, want nothing", kinds(envs))
				}
				return
			}
			if len(envs) != 1 {
				t.Fatalf("spooled %v, want one %s", kinds(envs), spool.KindAgentClose)
			}
			env, ac := envs[0], envs[0].AgentClose
			if env.ID != "agent-7:close:0" || ac.Type != c.want || ac.TranscriptChunk != "" ||
				!ac.StartedAt.Equal(testNow) {
				t.Errorf("envelope %s = %+v; want a %s without lines", env.ID, ac, c.want)
			}
		})
	}
}

// TestSubagentStopInternalAgent records an untyped agent with no meta
// file whose transcript has lines: an internal agent, not a main segment.
func TestSubagentStopInternalAgent(t *testing.T) {
	s := newSession(t, physicalTempDir(t))
	file := agentFile(s)
	writeFile(t, file, agentTranscript(s.cwd))
	s.mustRun("subagent_stop.json", "$AGENT_TRANSCRIPT", file, `"agent_type": "worker",`, "")
	ac := s.takeOne(spool.KindAgentClose).AgentClose
	if ac.Type != store.InternalAgent || ac.Lines.Read != 3 || ac.TranscriptChunk == "" ||
		len(s.logs) != 0 {
		t.Errorf("AgentClose = %+v, logs %q; want an internal agent with its lines", ac, s.logs)
	}
}

// TestSubagentStopTranscriptPath reads the agent transcript where the
// payload says, a leading ~ expanded, or where the harness keeps it when
// the payload names none. It records nothing for a path outside the
// session's subagents directory, a transcript_path that is missing or
// relative, or a session_id that is not one path element.
func TestSubagentStopTranscriptPath(t *testing.T) {
	outside := filepath.Join(physicalTempDir(t), "agent-agent-7.jsonl")
	writeFile(t, outside, agentTranscript("/work"))
	for _, c := range transcriptPathCases(outside) {
		t.Run(c.name, func(t *testing.T) {
			s := newSession(t, physicalTempDir(t))
			writeFile(t, agentFile(s), agentTranscript(s.cwd))
			s.mustRun("subagent_stop.json", "$AGENT_TRANSCRIPT", c.path(s), "sess-1", c.session)
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

// transcriptPathCase is one SubagentStop payload variant: the agent
// transcript path it names and its session_id.
type transcriptPathCase struct {
	name    string
	path    func(s *testSession) string
	session string // the payload's session_id
	want    string // the envelope id; "" for none
}

// transcriptPathCases are the payload variants of
// TestSubagentStopTranscriptPath; outside is an agent transcript outside
// every session's subagents directory.
func transcriptPathCases(outside string) []transcriptPathCase {
	none := func(*testSession) string { return "" }
	return []transcriptPathCase{
		{"named", agentFile, "sess-1", "agent-7:close:3"},
		{"empty field", none, "sess-1", "agent-7:close:3"},
		{"tilde", func(s *testSession) string {
			s.tb.Setenv("HOME", filepath.Dir(s.transcript))
			return "~/sess-1/subagents/agent-agent-7.jsonl"
		}, "sess-1", "agent-7:close:3"},
		{"outside", func(*testSession) string { return outside }, "sess-1", ""},
		{"climbs out", func(s *testSession) string {
			return filepath.Join(filepath.Dir(agentFile(s)), "..", "..", "x.jsonl")
		}, "sess-1", ""},
		{"symlink out", func(s *testSession) string {
			link := filepath.Join(filepath.Dir(agentFile(s)), "agent-link.jsonl")
			if err := os.Symlink(outside, link); err != nil {
				s.tb.Fatal(err)
			}
			return link
		}, "sess-1", ""},
		{"no transcript_path", func(s *testSession) string {
			s.transcript = ""
			return ""
		}, "sess-1", ""},
		{"relative transcript_path", func(s *testSession) string {
			s.transcript = "transcript.jsonl"
			return ""
		}, "sess-1", ""},
		{"session_id is ..", none, "..", ""},
		{"session_id with a separator", func(s *testSession) string {
			nested := filepath.Join(filepath.Dir(s.transcript), "nested", "sess-1")
			writeFile(s.tb, filepath.Join(nested, "subagents", "agent-agent-7.jsonl"),
				agentTranscript(s.cwd))
			return ""
		}, "nested/sess-1", ""},
	}
}
