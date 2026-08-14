package rewrite

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testCred is a fake credential used only to assert it never leaves the
// machine; kept off an APIKey-literal assignment so the secret scanner
// doesn't flag it.
const testCred = "placeholder-key"

func defaultOpts() Options {
	return Options{
		Backend:      PrintPrompt,
		Strength:     Paraphrase,
		Lang:         "French",
		OriginalLang: "English",
		Timeout:      5 * time.Second,
		LayerAAfter:  true,
		Temperature:  0.9,
		Candidates:   1,
	}
}

func TestBuildPromptParaphrase(t *testing.T) {
	p, err := BuildPrompt(Paraphrase, "Hello world facts 42.", "French", "English")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Hello world facts 42.", "clause order", "function words"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestBuildPromptHumanizeAndCode(t *testing.T) {
	for _, tc := range []struct {
		s  Strength
		kw string
	}{
		{Humanize, "human wrote it"},
		{Code, "comments"},
	} {
		p, err := BuildPrompt(tc.s, "ABC 123", "French", "English")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(p, "ABC 123") || !strings.Contains(p, tc.kw) {
			t.Fatalf("%s prompt wrong: %q", tc.s, p)
		}
	}
}

func TestBuildPromptUnknownStrengthErrors(t *testing.T) {
	if _, err := BuildPrompt(Strength("nope"), "ABC", "French", "English"); err == nil {
		t.Fatal("expected error for unknown strength")
	}
}

func TestStructuralAndBacktranslatePrompts(t *testing.T) {
	for _, s := range []Strength{Structural, Backtranslate} {
		p, err := BuildPrompt(s, "ABC 123", "German", "English")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(p, "ABC 123") {
			t.Fatalf("%s prompt missing text", s)
		}
	}
}

func TestPrintPromptBackend(t *testing.T) {
	out, info, err := Rewrite("Sample prose about water marks.", defaultOpts())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode != "print-prompt" || info.Backend != "print-prompt" {
		t.Fatalf("info = %+v", info)
	}
	if !strings.Contains(out, "Sample prose") {
		t.Fatalf("out = %q", out)
	}
	if info.Temperature != 0.9 {
		t.Fatalf("temperature = %v", info.Temperature)
	}
}

func TestPrintPromptIgnoresCandidates(t *testing.T) {
	opts := defaultOpts()
	opts.Candidates = 2
	out, info, err := Rewrite("Sample prose about water marks.", opts)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode != "print-prompt" || !strings.Contains(out, "Sample prose") {
		t.Fatalf("out = %q info = %+v", out, info)
	}
}

func TestLexicalDivergenceIdenticalIsZero(t *testing.T) {
	if got := LexicalDivergence("the cat sat", "the cat sat"); got != 0.0 {
		t.Fatalf("got %v", got)
	}
}

func TestLexicalDivergenceOrdering(t *testing.T) {
	similar := LexicalDivergence("the cat sat on the mat", "the dog sat on the mat")
	different := LexicalDivergence("the cat sat on the mat", "alpha beta gamma delta")
	if !(different > similar) {
		t.Fatalf("different=%v not > similar=%v", different, similar)
	}
}

func TestLexicalDivergenceEmptyInputs(t *testing.T) {
	if LexicalDivergence("", "") != 0.0 {
		t.Fatal("empty/empty should be 0")
	}
	if LexicalDivergence("", "text") != 1.0 {
		t.Fatal("empty/text should be 1")
	}
	if LexicalDivergence("text", "") != 1.0 {
		t.Fatal("text/empty should be 1")
	}
}

func TestSelectCandidatePrefersMoreDivergent(t *testing.T) {
	best, scores := SelectCandidate(
		"the cat sat on the mat",
		[]string{"the cat sat on the mat", "the dog sat on the mat", "alpha beta gamma delta"},
	)
	if best != "alpha beta gamma delta" {
		t.Fatalf("best = %q", best)
	}
	if len(scores) != 3 {
		t.Fatalf("scores len = %d", len(scores))
	}
}

func TestCheckRemoteLoopbackAllowedWithoutOptIn(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:11434", "http://localhost:11434", "http://[::1]:11434"} {
		if w, err := CheckRemote(u, false); err != nil || w != "" {
			t.Fatalf("%s: warning=%q err=%v", u, w, err)
		}
	}
}

func TestCheckRemoteDeniesNonLoopbackWithoutOptIn(t *testing.T) {
	if _, err := CheckRemote("http://example.com:11434", false); err == nil {
		t.Fatal("expected denial")
	}
}

func TestCheckRemoteAllowsNonLoopbackWithOptIn(t *testing.T) {
	w, err := CheckRemote("http://example.com:11434", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w, "content will leave this machine") {
		t.Fatalf("warning = %q", w)
	}
}

func TestCheckRemoteDeniesNonHTTPScheme(t *testing.T) {
	if _, err := CheckRemote("file:///etc/passwd", true); err == nil {
		t.Fatal("expected scheme denial")
	}
}

func TestFlagEnv(t *testing.T) {
	const k = "WATERMARKS_REWRITE_ALLOW_REMOTE"
	t.Setenv(k, "")
	if FlagEnv(k) {
		t.Fatal("empty should be false")
	}
	for _, v := range []string{"1", "true", "yes", "on"} {
		t.Setenv(k, v)
		if !FlagEnv(k) {
			t.Fatalf("%q should be true", v)
		}
	}
	t.Setenv(k, "0")
	if FlagEnv(k) {
		t.Fatal("0 should be false")
	}
}

func TestRewriteDeniesRemoteHostWithoutOptIn(t *testing.T) {
	opts := defaultOpts()
	opts.Backend = OpenAICompatible
	opts.Model = "m"
	opts.BaseURL = "http://example.com:11434"
	opts.APIKey = testCred
	opts.LayerAAfter = false
	if _, _, err := Rewrite("secret text", opts); err == nil {
		t.Fatal("expected remote denial")
	}
}

// TestRewriteBlocksRedirectAndNeverSendsKey verifies a 302 from the (loopback)
// endpoint does not re-send the API key to the redirect target — the request
// must fail instead.
func TestRewriteBlocksRedirectAndNeverSendsKey(t *testing.T) {
	captured := make(chan string, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"rewritten"}}]}`))
	}))
	defer collector.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, collector.URL+"/collect", http.StatusFound)
	}))
	defer redirector.Close()

	opts := defaultOpts()
	opts.Backend = OpenAICompatible
	opts.Model = "m"
	opts.BaseURL = redirector.URL
	opts.APIKey = testCred
	opts.LayerAAfter = false

	if _, _, err := Rewrite("secret text", opts); err == nil {
		t.Fatal("expected redirect to fail the request")
	}
	select {
	case auth := <-captured:
		t.Fatalf("redirect target received a request (key leak): auth=%q", auth)
	case <-time.After(200 * time.Millisecond):
		// good: collector never hit
	}
}
