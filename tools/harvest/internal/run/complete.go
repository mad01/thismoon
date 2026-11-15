package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// completeTimeout bounds the model call. A 200 KB digest through a slow
// route takes minutes; a call that runs past this has hung.
const completeTimeout = 10 * time.Minute

// Call is what call.json records about the model call, for the done line.
type Call struct {
	Backend          string  `json:"backend"`
	Model            string  `json:"model"`
	CostUSD          float64 `json:"cost_usd"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
}

// CompleteConfig is what Complete needs besides the session id.
type CompleteConfig struct {
	State *state.Dir
	// Select resolves the backend: llm.Select outside tests.
	Select func(llm.Options) (llm.Backend, error)
	// Options steer Select. Observe is set by Complete.
	Options llm.Options
}

// Complete is the complete step: one model call with the job's system.md as
// the system prompt and its user.md as the user text. It writes the reply
// to reply.txt as it came, without reading it, and the call's backend,
// model, cost and tokens to call.json.
func Complete(ctx context.Context, cfg CompleteConfig, session string) error {
	job, err := cfg.State.Job(session)
	if err != nil {
		return err
	}
	system, err := os.ReadFile(job.System())
	if err != nil {
		return fmt.Errorf("run: read the system prompt: %w", err)
	}
	user, err := os.ReadFile(job.User())
	if err != nil {
		return fmt.Errorf("run: read the user text: %w", err)
	}

	var info llm.CallInfo
	opts := cfg.Options
	opts.Observe = func(ci llm.CallInfo) { info = ci }
	backend, err := cfg.Select(opts)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, completeTimeout)
	defer cancel()
	reply, err := backend.Complete(ctx, string(system), string(user))
	if err != nil {
		return err
	}

	if err := os.WriteFile(job.Reply(), []byte(reply), 0o600); err != nil {
		return fmt.Errorf("run: write the reply: %w", err)
	}
	call := Call{
		Backend:          backend.Name(),
		Model:            info.Model,
		CostUSD:          info.CostUSD,
		PromptTokens:     info.PromptTokens,
		CompletionTokens: info.CompletionTokens,
	}
	if call.Model == "" {
		call.Model = backend.Model()
	}
	b, err := json.Marshal(call)
	if err != nil {
		return fmt.Errorf("run: encode the call: %w", err)
	}
	if err := os.WriteFile(job.Call(), append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("run: write the call: %w", err)
	}
	return nil
}
