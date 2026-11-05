package mcpserver

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/doctor"
)

func TestBrainDoctorIsReadOnly(t *testing.T) {
	for _, tool := range mcptest.ListTools(t, New("test", Options{})) {
		if tool.Name != brain.Facts().MCPDoctorTool {
			continue
		}
		a := tool.Annotations
		if a == nil || !a.ReadOnlyHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Fatalf("brain_doctor annotations = %+v, want read-only and closed-world", a)
		}
		return
	}
	t.Fatal("server does not list brain_doctor")
}

func TestInstructionsNameTheDoctorTool(t *testing.T) {
	got := agentdoc.Instructions(brain.Facts())
	if !strings.Contains(got, "'brain_doctor'") || !strings.Contains(got, "'brain doctor'") {
		t.Errorf("instructions do not point at both doctors:\n%s", got)
	}
}

func TestBrainDoctorReport(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()
	h := testServer(t)

	_, report, err := h.handleDoctor(ctx, nil, struct{}{})
	if err != nil {
		t.Fatalf("a failing check must be a result, not a tool error: %v", err)
	}
	if report.OK || statusOf(report, "index") != doctor.StatusFail {
		t.Fatalf("before indexing: ok=%v index=%q, want the index check to fail",
			report.OK, statusOf(report, "index"))
	}
	indexDir, err := brain.IndexDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(indexDir); !os.IsNotExist(err) {
		t.Errorf("doctor created %s; it must leave nothing behind", indexDir)
	}

	cfg, err := h.loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ensureIndex(cfg); err != nil {
		t.Fatal(err)
	}
	if _, report, _ = h.handleDoctor(ctx, nil, struct{}{}); !report.OK {
		t.Errorf("after indexing: %+v, want every check to pass", report.Checks)
	}
}

func statusOf(r doctor.Report, name string) string {
	for _, c := range r.Checks {
		if c.Name == name {
			return c.Status
		}
	}
	return ""
}
