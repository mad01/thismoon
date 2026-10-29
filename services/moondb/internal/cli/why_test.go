package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/query"
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
			BriefChunk: servertest.AgentBriefSHA, Recorded: true,
		}, []string{"agent:    reviewer (general-purpose)  [agent-rev]", `brief:    "review it"`}},
		{
			"recorded before briefs",
			query.WhyAgent{ID: "a-old", Type: "Explore", Recorded: true},
			[]string{
				"agent:    Explore  [a-old]",
				"brief:    unknown, recorded before moondb kept briefs",
			},
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
