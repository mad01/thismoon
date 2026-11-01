package ask

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/mad01/thismoon/kit/llm"
)

// fakeLLM stands in for a provider: each call reports fixed usage to its
// observer, and a call whose message names a file under failRepo fails.
type fakeLLM struct {
	mu       sync.Mutex
	messages []string
	failRepo string
	noWorker error
}

func (f *fakeLLM) worker(observe func(llm.CallInfo)) (llm.Backend, error) {
	if f.noWorker != nil {
		return nil, f.noWorker
	}
	return &fakeBackend{f: f, observe: observe}, nil
}

func (f *fakeLLM) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

type fakeBackend struct {
	f       *fakeLLM
	observe func(llm.CallInfo)
}

func (b *fakeBackend) Name() string  { return "fake" }
func (b *fakeBackend) Model() string { return "claude-haiku-5-5" }

func (b *fakeBackend) Complete(_ context.Context, system, user string) (string, error) {
	b.f.mu.Lock()
	b.f.messages = append(b.f.messages, user)
	b.f.mu.Unlock()
	if system != SystemPrompt {
		return "", errors.New("fake: unexpected system prompt")
	}
	if b.observe != nil {
		b.observe(llm.CallInfo{PromptTokens: 1_000_000, CompletionTokens: 100_000})
	}
	if b.f.failRepo != "" && strings.Contains(user, `path="`+b.f.failRepo+"/") {
		return "", errors.New("fake: provider down")
	}
	return fmt.Sprintf("  - %d files read  \n", strings.Count(user, "<file ")), nil
}

// corpusOf builds a corpus of one-line files without touching disk.
func corpusOf(hits ...Hit) Corpus {
	var c Corpus
	for _, h := range hits {
		c.Files = append(c.Files, CorpusFile{Hit: h, Lines: 1, text: []string{"x"}, bytes: 2})
		c.Lines++
		c.Chars += 2
	}
	c.FilesTotal = len(hits)
	return c
}

// haikuCost is the list price of one fake call: 1M tokens in, 100k out.
const haikuCost = 0.10 + 0.05

func TestRunOneCall(t *testing.T) {
	f := &fakeLLM{}
	corpus := corpusOf(Hit{"o/a", "x.go"}, Hit{"o/b", "y.go"})
	res, err := Run(context.Background(), Request{Question: "q?", Corpus: corpus}, f.worker)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls() != 1 {
		t.Fatalf("%d worker calls, want 1", f.calls())
	}
	if res.Answer != "- 2 files read" {
		t.Errorf("answer = %q, want the trimmed reply", res.Answer)
	}
	if res.Backend != "fake" || res.Model != "claude-haiku-5-5" || res.PerRepo != nil {
		t.Errorf("backend/model/per_repo = %q/%q/%v", res.Backend, res.Model, res.PerRepo)
	}
	if res.Usage.PromptTokens != 1_000_000 || math.Abs(res.Usage.CostUSD-haikuCost) > 1e-9 {
		t.Errorf("usage = %+v, want 1M prompt tokens at $%.2f", res.Usage, haikuCost)
	}
	if !strings.Contains(f.messages[0], `<file path="o/a/x.go" lines="1">`) {
		t.Errorf("message does not name files org/repo/path:\n%s", f.messages[0])
	}
	want := "corpus: 2 files, 2 lines; fake/claude-haiku-5-5; $0.150"
	if got := res.Footer(); got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

func TestRunPerRepo(t *testing.T) {
	f := &fakeLLM{failRepo: "o/b"}
	corpus := corpusOf(
		Hit{"o/c", "1.go"},
		Hit{"o/a", "2.go"},
		Hit{"o/c", "3.go"},
		Hit{"o/b", "4.go"},
	)
	res, err := Run(
		context.Background(),
		Request{Question: "q?", Corpus: corpus, PerRepo: true},
		f.worker,
	)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls() != 3 {
		t.Fatalf("%d worker calls, want one per repo (3)", f.calls())
	}
	var repos []string
	for _, a := range res.PerRepo {
		repos = append(repos, a.Repo)
	}
	if got := strings.Join(repos, ","); got != "o/c,o/a,o/b" {
		t.Errorf("per-repo order = %s, want repo rank order o/c,o/a,o/b", got)
	}
	if res.PerRepo[0].Answer != "- 2 files read" || res.PerRepo[2].Error == "" {
		t.Errorf("per_repo = %+v, want o/c over 2 files and o/b failed", res.PerRepo)
	}
	wantAnswer := "## o/c\n\n- 2 files read\n\n## o/a\n\n- 1 files read\n\n## o/b\n\nerror: "
	if !strings.HasPrefix(res.Answer, wantAnswer) {
		t.Errorf("answer = %q, want it to start %q", res.Answer, wantAnswer)
	}
	if math.Abs(res.Usage.CostUSD-3*haikuCost) > 1e-9 {
		t.Errorf("cost = %f, want the three calls summed", res.Usage.CostUSD)
	}
}

func TestRunPerRepoAllFail(t *testing.T) {
	f := &fakeLLM{failRepo: "o/a"}
	_, err := Run(
		context.Background(),
		Request{Question: "q?", Corpus: corpusOf(Hit{"o/a", "x.go"}), PerRepo: true},
		f.worker,
	)
	if err == nil || !strings.Contains(err.Error(), "every per-repo worker call failed") {
		t.Errorf("err = %v, want every call failed", err)
	}
}

func TestRunRefusesBeforeAnyCall(t *testing.T) {
	f := &fakeLLM{}
	if _, err := Run(context.Background(), Request{Question: "q?"}, f.worker); err == nil {
		t.Error("an empty corpus ran, want an error")
	}
	f.noWorker = llm.ErrNoBackend
	_, err := Run(
		context.Background(),
		Request{Question: "q?", Corpus: corpusOf(Hit{"o/a", "x"})},
		f.worker,
	)
	if !errors.Is(err, llm.ErrNoBackend) {
		t.Errorf("err = %v, want ErrNoBackend", err)
	}
	if f.calls() != 0 {
		t.Errorf("%d worker calls, want none", f.calls())
	}
}

func TestSettingsValidate(t *testing.T) {
	ok := []Settings{
		{Model: "m", Effort: "low"},
		{Backend: llm.Claude, Model: "m", Effort: "max"},
		{Backend: llm.LiteLLM, Model: "m", Effort: "xhigh"},
	}
	for _, s := range ok {
		if err := s.Validate(); err != nil {
			t.Errorf("%+v: %v, want valid", s, err)
		}
	}
	bad := map[string]Settings{
		"ask.backend": {Backend: "vertex", Model: "m", Effort: "low"},
		"ask.effort":  {Model: "m", Effort: "turbo"},
	}
	for key, s := range bad {
		if err := s.Validate(); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%+v: %v, want an error naming %s", s, err, key)
		}
	}
}

func TestNewWorkerNamesWhatToSet(t *testing.T) {
	for _, v := range llm.ProviderEnv {
		t.Setenv(v, "")
	}
	t.Setenv("PATH", t.TempDir()) // no claude CLI either
	_, err := NewWorker(Settings{Model: "m", Effort: "low"}, nil)
	if !errors.Is(err, llm.ErrNoBackend) {
		t.Fatalf("err = %v, want ErrNoBackend", err)
	}
	for _, want := range []string{"LITELLM_BASE_URL", "claude on PATH", "OPENROUTER_API_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

func TestCostUSD(t *testing.T) {
	if got := CostUSD("claude-sonnet-5-5", 1_000_000, 1_000_000); math.Abs(got-12) > 1e-9 {
		t.Errorf("sonnet 1M/1M = %f, want 12", got)
	}
	if got := CostUSD("anthropic/claude-haiku-5.5", 1_000_000, 0); math.Abs(got-0.10) > 1e-9 {
		t.Errorf("OpenRouter haiku id = %f, want it priced as claude-haiku-5-5", got)
	}
	if got := CostUSD("some-local-alias", 1_000_000, 1_000_000); got != 0 {
		t.Errorf("unknown model = %f, want 0", got)
	}
}
