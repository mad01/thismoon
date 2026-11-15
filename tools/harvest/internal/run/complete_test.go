package run

import (
	"context"
	"os"
	"testing"

	"github.com/mad01/thismoon/kit/llm"
)

// quietBackend reports no CallInfo, as a backend that saw no response does.
type quietBackend struct{ fakeBackend }

func (q *quietBackend) Complete(_ context.Context, _, _ string) (string, error) {
	return "the reply, unread", nil
}

func TestCompleteWritesTheReplyAndTheCall(t *testing.T) {
	f := newFixture(t)
	job, err := f.state.Job("s1")
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Reset(); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{job.System(): "sys", job.User(): "usr"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var gotOpts llm.Options
	q := &quietBackend{}
	cfg := CompleteConfig{
		State:   f.state,
		Options: llm.Options{Backend: llm.LiteLLM, Model: "m1", Effort: "low", AllowCLI: true},
		Select: func(o llm.Options) (llm.Backend, error) {
			gotOpts = o
			return q, nil
		},
	}
	if err := Complete(context.Background(), cfg, "s1"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if gotOpts.Backend != llm.LiteLLM || gotOpts.Model != "m1" || gotOpts.Effort != "low" ||
		!gotOpts.AllowCLI || gotOpts.Observe == nil {
		t.Errorf("Select got %+v, want the configured options and an observer", gotOpts)
	}
	reply, err := os.ReadFile(job.Reply())
	if err != nil || string(reply) != "the reply, unread" {
		t.Errorf("reply.txt = %q, %v", reply, err)
	}
	call, err := readCall(job.Call())
	if err != nil {
		t.Fatal(err)
	}
	if call != (Call{Backend: "fake", Model: "asked-model"}) {
		t.Errorf("call.json = %+v, want the backend's own model when none was observed", call)
	}
}
