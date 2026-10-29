package cli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/buildinfo"

	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// TestPrintWhyShowsTheBrief: an edit a subagent made prints the agent and
// the first line of its brief beside the turn's prompt, and says why a
// brief is unknown when it is.
func TestPrintWhyShowsTheBrief(t *testing.T) {
	c := traceClient(t)
	turn := &store.Turn{ID: servertest.NewTurn, PromptChunk: servertest.PromptSHA}
	cases := []struct {
		name  string
		agent query.WhyAgent
		want  []string
	}{
		{"recorded with a brief", query.WhyAgent{
			ID: servertest.AgentID, Type: "general-purpose", Name: servertest.AgentName,
			BriefChunk: servertest.AgentBriefSHA, LaterMessages: 2, Recorded: true,
		}, []string{
			"agent:    reviewer (general-purpose)  [agent-rev]", `brief:    "review it"`,
			"later:    2 messages from its parent after the spawn brief",
		}},
		{
			"recorded before briefs",
			query.WhyAgent{ID: "a-old", Type: "Explore", Recorded: true},
			[]string{
				"agent:    Explore  [a-old]",
				"brief:    unknown, recorded before moondb kept briefs; moondb import --since 30d --force",
			},
		},
		{
			"type internal",
			query.WhyAgent{ID: "a-int", Type: store.InternalAgent, Recorded: true},
			[]string{"brief:    unknown, type internal, no transcript"},
		},
		{
			"known from its edits",
			query.WhyAgent{ID: "a-gone", Type: "worker"},
			[]string{"brief:    unknown, no agent record"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := query.WhyResult{
				Path: "a.go", Line: 3, Method: query.MethodContent, Turn: turn, Agent: &tc.agent,
				Edit: &store.Edit{
					ID: "e1:0", Tool: "Edit", Kind: store.EditUpdate,
					AgentID: tc.agent.ID, AgentType: tc.agent.Type,
				},
			}
			var out bytes.Buffer
			printWhy(context.Background(), &out, c, res)
			for _, want := range append(tc.want, `turn:     turn-new  "add the server"`) {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output lacks %q:\n%s", want, out.String())
				}
			}
		})
	}
}

// TestPrintWhyShowsTheLastMessage: an edit made after its subagent got a
// later message prints that message's first line, when it arrived and who
// sent it, under the brief. With none before the edit the count says so
// when the record keeps its messages, and says none is shown when it
// does not.
func TestPrintWhyShowsTheLastMessage(t *testing.T) {
	st := servertest.NewStore()
	key := st.AddChunk([]byte("Tighten the marker scan to command values.\nThen land."))
	ts := httptest.NewServer(server.New(st, servertest.NewQuerier(st), buildinfo.Info{}).Handler())
	t.Cleanup(ts.Close)
	c := client.New(ts.URL)
	at := time.Date(2026, 10, 29, 20, 9, 57, 0, time.Local)
	agent := query.WhyAgent{
		ID: servertest.AgentID, Type: "general-purpose", Name: servertest.AgentName,
		BriefChunk: servertest.AgentBriefSHA, LaterMessages: 3, Recorded: true,
	}
	cases := []struct {
		name string
		last *store.AgentMessage
		kept bool
		want []string
	}{
		{
			"one before the edit",
			&store.AgentMessage{At: at, From: "team-lead", Chunk: key}, true,
			[]string{
				`last message: "Tighten the marker scan to command values."  ` +
					`(2026-10-29 20:09 from team-lead, before this edit)`,
				"later:    3 messages from its parent after the spawn brief\n",
			},
		},
		{"none before the edit", nil, true, []string{
			"later:    3 messages from its parent after the spawn brief, none before this edit\n",
		}},
		{"no messages kept", nil, false, []string{
			"later:    3 messages from its parent after the spawn brief, none shown\n",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := agent
			a.LastMessage, a.MessagesKept = tc.last, tc.kept
			res := query.WhyResult{
				Path: "a.go", Line: 3, Method: query.MethodContent, Agent: &a,
				Edit: &store.Edit{ID: "e1:0", Tool: "Edit", Kind: store.EditUpdate, AgentID: a.ID},
			}
			var out bytes.Buffer
			printWhy(context.Background(), &out, c, res)
			for _, want := range append(tc.want, `brief:    "review it"`) {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output lacks %q:\n%s", want, out.String())
				}
			}
			if tc.last == nil && strings.Contains(out.String(), "last message") {
				t.Errorf("output names a last message:\n%s", out.String())
			}
		})
	}
}
