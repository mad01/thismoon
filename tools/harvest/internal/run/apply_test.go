package run

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/harvest/internal/reply"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// job sets up the job dir apply reads: the counts, the call and the reply.
func (f *fixture) job(session, replyText string) state.Job {
	f.t.Helper()
	job, err := f.state.Job(session)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := job.Reset(); err != nil {
		f.t.Fatal(err)
	}
	if err := job.SetCounts(state.Counts{Lines: 12, DigestBytes: 5000}); err != nil {
		f.t.Fatal(err)
	}
	for path, content := range map[string]string{
		job.Call():  `{"backend":"claude","model":"claude-haiku-5-5","cost_usd":0}`,
		job.Reply(): replyText,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			f.t.Fatal(err)
		}
	}
	return job
}

func (f *fixture) apply(session string) error {
	return Apply(context.Background(), ApplyConfig{State: f.state, Brain: f.brain}, session)
}

func TestApplyReportsEachOutcome(t *testing.T) {
	f := newFixture(t)
	f.job("s1", "Here you go:\n```json\n"+`{"captures":[
		{"title":"kept claim","body":"b1","tags":["go"],"source":"https://example.com/a"},
		{"title":"dup claim","body":"b2"},
		{"title":"secret claim","body":"b3"},
		{"title":"broken claim","body":"b4"}],
	 "dropped":[{"candidate":"a plan","reason":"work state"}]}`+"\n```\n")
	if err := f.apply("s1"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	want := "done s1: model=claude-haiku-5-5 digest=5000B cost=? backend=claude sandbox=off\n" +
		"  captured: main:inbox/note.md\n" +
		"  dropped: likely duplicate: dup claim\n" +
		"  dropped: secret found\n" +
		"  failed: broken claim: brain capture: the vault is gone\n" +
		"  dropped by the model: 1\n" +
		"    work state\n"
	log := f.log()
	if !strings.HasSuffix(log, want) {
		t.Errorf("log =\n%s\nwant it to end with\n%s", log, want)
	}
	// brain scans the title too, and the model drops secrets: neither the
	// refused title nor a dropped candidate may reach the log.
	for _, leak := range []string{"secret claim", "a plan"} {
		if strings.Contains(log, leak) {
			t.Errorf("the log carries %q:\n%s", leak, log)
		}
	}
	if got := f.mark("s1"); got != 12 {
		t.Errorf("mark = %d, want the line count prepare recorded, 12", got)
	}
	if calls := f.read("calls"); !strings.Contains(calls,
		"--title=kept claim\n--tag=go\n--tag=harvest\n--by=agent:s1\n"+
			"--source=https://example.com/a\n-o\njson\n") {
		t.Errorf("brain calls lack the first capture's arguments:\n%s", calls)
	}
}

func TestApplyPassesATitleThatLooksLikeAFlag(t *testing.T) {
	f := newFixture(t)
	f.job("s1", `{"captures":[{"title":"--vault=other","body":"b"}]}`)
	if err := f.apply("s1"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if calls := f.read("calls"); !strings.HasPrefix(calls, "capture\n--title=--vault=other\n") {
		t.Errorf("brain calls =\n%s\nwant the title as one --title= argument", calls)
	}
}

func TestApplyWithNothingToKeep(t *testing.T) {
	f := newFixture(t)
	f.job("s1", `{"captures":[],"dropped":[{"candidate":"a guess","reason":"no evidence"}]}`)
	if err := f.apply("s1"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !strings.HasSuffix(f.log(),
		"\n  captured: none\n  dropped by the model: 1\n    no evidence\n") {
		t.Errorf("log lacks captured: none and the model's drop:\n%s", f.log())
	}
	if strings.Contains(f.log(), "a guess") {
		t.Errorf("the log carries the dropped candidate:\n%s", f.log())
	}
	if f.read("calls") != "" {
		t.Error("brain was called with nothing to capture")
	}
	if got := f.mark("s1"); got != 12 {
		t.Errorf("mark = %d, want 12", got)
	}
}

func TestApplyCountsEveryModelDrop(t *testing.T) {
	f := newFixture(t)
	drops := make([]string, reply.MaxReasons+2)
	for i := range drops {
		drops[i] = `{"candidate":"c","reason":"r"}`
	}
	f.job("s1", `{"captures":[],"dropped":[`+strings.Join(drops, ",")+`]}`)
	if err := f.apply("s1"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	want := fmt.Sprintf("\n  dropped by the model: %d (the first %d reasons)\n",
		reply.MaxReasons+2, reply.MaxReasons)
	if log := f.log(); !strings.Contains(log, want) ||
		strings.Count(log, "    r\n") != reply.MaxReasons {
		t.Errorf("log =\n%s\nwant %q and %d reasons", log, want, reply.MaxReasons)
	}
}

func TestApplyFailsOnlyWhenEveryCaptureFails(t *testing.T) {
	f := newFixture(t)
	f.job("s1", `{"captures":[{"title":"broken","body":"b"},{"title":"dup","body":"b"}]}`)
	if err := f.apply("s1"); err != nil {
		t.Fatalf("Apply() error = %v; a refusal is not a failure", err)
	}

	g := newFixture(t)
	g.job("s2", `{"captures":[{"title":"broken","body":"b"}]}`)
	if err := g.apply("s2"); err == nil {
		t.Fatal("Apply() error = nil, want the failure")
	}
	if got := g.mark("s2"); got != 0 {
		t.Errorf("a failed apply wrote mark %d", got)
	}
}
