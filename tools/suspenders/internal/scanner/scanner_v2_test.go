package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// shannonEntropy
// ---------------------------------------------------------------------------

func TestShannonEntropy(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  float64
	}{
		{name: "empty string", input: "", want: 0},
		{name: "single repeated char", input: "aaaaaaaa", want: 0},
		{name: "four distinct chars", input: "abcd", want: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shannonEntropy(tc.input)
			if got != tc.want {
				t.Errorf("shannonEntropy(%q) = %f, want %f", tc.input, got, tc.want)
			}
		})
	}

	t.Run("random token scores higher than english word", func(t *testing.T) {
		random := shannonEntropy("xK9mQ2vR7pL4wT8zB6yC3nD1")
		english := shannonEntropy("administratorpassword")
		if random <= english {
			t.Errorf(
				"expected random token entropy (%f) > english word entropy (%f)",
				random,
				english,
			)
		}
	})
}

// ---------------------------------------------------------------------------
// New and fixed rule patterns
// ---------------------------------------------------------------------------

func TestNewRulePatterns(t *testing.T) {
	cases := []struct {
		name      string
		ruleID    string
		input     string
		wantMatch bool
	}{
		// private-key-header — previously missed variants
		{
			name:      "private-key-header encrypted PKCS8 matches",
			ruleID:    "private-key-header",
			input:     "-----BEGIN ENCRYPTED PRIVATE KEY-----",
			wantMatch: true,
		},
		{
			name:      "private-key-header PGP block matches",
			ruleID:    "private-key-header",
			input:     "-----BEGIN PGP PRIVATE KEY BLOCK-----",
			wantMatch: true,
		},
		{
			name:      "private-key-header SSH2 encrypted matches",
			ruleID:    "private-key-header",
			input:     "-----BEGIN SSH2 ENCRYPTED PRIVATE KEY-----",
			wantMatch: true,
		},
		{
			name:      "private-key-header no match on PGP public key block",
			ruleID:    "private-key-header",
			input:     "-----BEGIN PGP PUBLIC KEY BLOCK-----",
			wantMatch: false,
		},

		// PuTTY private key
		{
			name:      "putty-private-key matches",
			ruleID:    "putty-private-key",
			input:     "PuTTY-User-Key-File-3: ssh-ed25519",
			wantMatch: true,
		},

		// GitHub refresh token — previously missing from the gh[...]_ class
		{
			name:      "github-pat-classic ghr_ matches",
			ruleID:    "github-pat-classic",
			input:     "ghr_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			wantMatch: true,
		},

		// Slack app-level token
		{
			name:      "slack-app-level-token matches",
			ruleID:    "slack-app-level-token",
			input:     "xapp-1-A052ABCDEFG-1234567890123-" + strings.Repeat("a1", 32),
			wantMatch: true,
		},
		{
			name:      "slack-app-level-token no match on xoxb",
			ruleID:    "slack-app-level-token",
			input:     "xoxb-123456789-abcdefgh",
			wantMatch: false,
		},

		// Stripe webhook secret
		{
			name:      "stripe-webhook-secret matches",
			ruleID:    "stripe-webhook-secret",
			input:     "whsec_" + strings.Repeat("a", 32),
			wantMatch: true,
		},

		// GitLab runner and deploy tokens
		{
			name:      "gitlab-runner-token matches",
			ruleID:    "gitlab-runner-token",
			input:     "glrt-" + strings.Repeat("a", 20),
			wantMatch: true,
		},
		{
			name:      "gitlab-deploy-token matches",
			ruleID:    "gitlab-deploy-token",
			input:     "gldt-" + strings.Repeat("a", 20),
			wantMatch: true,
		},

		// Groq
		{
			name:      "groq-api-key matches",
			ruleID:    "groq-api-key",
			input:     "gsk_" + strings.Repeat("a", 52),
			wantMatch: true,
		},
		{
			name:      "groq-api-key no match on short value",
			ruleID:    "groq-api-key",
			input:     "gsk_short",
			wantMatch: false,
		},

		// Tailscale
		{
			name:      "tailscale-key auth matches",
			ruleID:    "tailscale-key",
			input:     "tskey-auth-kFGiAS7CNTRL-2dpMswsLF8UdDydPRMiWDUEXAMPLE",
			wantMatch: true,
		},
		{
			name:      "tailscale-key no match on other prefix",
			ruleID:    "tailscale-key",
			input:     "tskey-other-abcdefghijkl",
			wantMatch: false,
		},

		// Dynatrace
		{
			name:      "dynatrace-api-token matches",
			ruleID:    "dynatrace-api-token",
			input:     "dt0c01." + strings.Repeat("A", 24) + "." + strings.Repeat("B", 64),
			wantMatch: true,
		},

		// age
		{
			name:      "age-secret-key matches",
			ruleID:    "age-secret-key",
			input:     "AGE-SECRET-KEY-1" + strings.Repeat("Q", 58),
			wantMatch: true,
		},

		// Azure storage account key
		{
			name:      "azure-storage-account-key matches",
			ruleID:    "azure-storage-account-key",
			input:     `AccountKey=` + strings.Repeat("a", 86) + "==",
			wantMatch: true,
		},
		{
			name:      "azure-storage-account-key no match without keyword",
			ruleID:    "azure-storage-account-key",
			input:     strings.Repeat("a", 86) + "==",
			wantMatch: false,
		},

		// URL basic auth — any scheme, not just databases
		{
			name:      "url-basic-auth https matches",
			ruleID:    "url-basic-auth",
			input:     "https://deploy:s3cr3tpass@registry.example.com/v2",
			wantMatch: true,
		},
		{
			name:      "url-basic-auth amqp matches",
			ruleID:    "url-basic-auth",
			input:     "amqp://guest:guestpw@rabbitmq:5672/",
			wantMatch: true,
		},
		{
			name:      "url-basic-auth no match on env placeholder",
			ruleID:    "url-basic-auth",
			input:     "https://${USER}:${PASS}@host/path",
			wantMatch: false,
		},
		{
			name:      "url-basic-auth no match without credentials",
			ruleID:    "url-basic-auth",
			input:     "https://example.com/path",
			wantMatch: false,
		},

		// Unquoted assignment — .env / YAML style
		{
			name:      "unquoted-secret-assignment env style matches",
			ruleID:    "unquoted-secret-assignment",
			input:     "DB_PASSWORD=xK9mQ2vR7pL4wT8z",
			wantMatch: true,
		},
		{
			name:      "unquoted-secret-assignment yaml style matches",
			ruleID:    "unquoted-secret-assignment",
			input:     "api_key: xK9mQ2vR7pL4wT8z",
			wantMatch: true,
		},
		{
			name:      "unquoted-secret-assignment no match on variable reference",
			ruleID:    "unquoted-secret-assignment",
			input:     "PASSWORD=${SECRET_FROM_VAULT}",
			wantMatch: false,
		},
		{
			name:      "unquoted-secret-assignment no match on function call",
			ruleID:    "unquoted-secret-assignment",
			input:     "password = get_password(env)",
			wantMatch: false,
		},

		// Discord — tightened to [MNO] first char so JWTs no longer collide
		{
			name:      "discord-bot-token no match on jwt-shaped string",
			ruleID:    "discord-bot-token",
			input:     "eyJhbGciOiJIUzI1NiIsInR5cCI.eyJzdW.abcdefghijklmnopqrstuvwxyzAB",
			wantMatch: false,
		},
	}

	ruleMap := make(map[string]Rule, len(DefaultRules))
	for _, r := range DefaultRules {
		ruleMap[r.ID] = r
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule, ok := ruleMap[tc.ruleID]
			if !ok {
				t.Fatalf("rule %q not found in DefaultRules", tc.ruleID)
			}
			got := rule.Pattern.MatchString(tc.input)
			if got != tc.wantMatch {
				if tc.wantMatch {
					t.Errorf("expected rule %q to match %q but it did not", tc.ruleID, tc.input)
				} else {
					t.Errorf("expected rule %q NOT to match %q but it did", tc.ruleID, tc.input)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Entropy gating
// ---------------------------------------------------------------------------

func TestEntropyGating(t *testing.T) {
	scan := func(t *testing.T, name, content string) []Finding {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		sc := New(DefaultRules)
		findings, err := sc.ScanFile(path)
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		return findings
	}

	t.Run("repeated-char placeholder password is not flagged", func(t *testing.T) {
		findings := scan(t, "app.env", "PASSWORD=aaaaaaaaaa\n")
		if len(findings) != 0 {
			t.Errorf("expected no findings for zero-entropy value, got %+v", findings)
		}
	})

	t.Run("random unquoted password is flagged", func(t *testing.T) {
		findings := scan(t, "app.env", "PASSWORD=xK9mQ2vR7pL4wT8z\n")
		found := false
		for _, f := range findings {
			if f.Rule.ID == "unquoted-secret-assignment" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected unquoted-secret-assignment finding, got %+v", findings)
		}
	})

	t.Run("high-entropy blob is flagged", func(t *testing.T) {
		blob := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN012345"
		findings := scan(t, "config.go", "data := \""+blob+"\"\n")
		found := false
		for _, f := range findings {
			if f.Rule.ID == "high-entropy-string" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected high-entropy-string finding, got %+v", findings)
		}
	})

	t.Run("high-entropy rule skips lockfiles", func(t *testing.T) {
		blob := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN012345"
		findings := scan(t, "package-lock.json", "\"integrity\": \""+blob+"\"\n")
		for _, f := range findings {
			if f.Rule.ID == "high-entropy-string" {
				t.Errorf("high-entropy-string should not fire in package-lock.json")
			}
		}
	})

	t.Run("high-entropy rule skips SRI hashes and ssh public keys", func(t *testing.T) {
		content := "integrity = \"sha512-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN012345\"\n" +
			"AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhAbcDefGhi\n"
		findings := scan(t, "notes.txt", content)
		for _, f := range findings {
			if f.Rule.ID == "high-entropy-string" {
				t.Errorf("high-entropy-string should not fire on %q", f.Context)
			}
		}
	})

	t.Run("high-entropy rule defers to specific rules on the same span", func(t *testing.T) {
		token := "ghp_Qw7eRt9yUi2oPa4sDf6gHj8kLz0xCv1bNm3M"
		findings := scan(t, "main.go", "token := \""+token+"\"\n")
		var ids []string
		for _, f := range findings {
			ids = append(ids, f.Rule.ID)
			if f.Rule.ID == "high-entropy-string" {
				t.Errorf(
					"high-entropy-string should defer to github-pat-classic, got rules %v",
					ids,
				)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Inline ignore marker
// ---------------------------------------------------------------------------

func TestInlineIgnore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	content := "key := \"AKIA1234567890ABCDEF\" // suspenders:ignore\n" +
		"other := \"AKIA1234567890ABCDEF\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	sc := New(DefaultRules)
	findings, err := sc.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile error: %v", err)
	}
	for _, f := range findings {
		if f.Line == 1 {
			t.Errorf("line 1 carries suspenders:ignore and should not be flagged: %+v", f)
		}
	}
	found := false
	for _, f := range findings {
		if f.Line == 2 && f.Rule.ID == "aws-access-key-id" {
			found = true
		}
	}
	if !found {
		t.Error("expected aws-access-key-id finding on line 2 without ignore marker")
	}
}

// ---------------------------------------------------------------------------
// Multiple matches per line and cross-redaction
// ---------------------------------------------------------------------------

func TestMultipleMatchesPerLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.txt")
	content := "AKIA1234567890ABCDEF AKIAFEDCBA0987654321\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	sc := New(DefaultRules)
	findings, err := sc.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile error: %v", err)
	}
	count := 0
	for _, f := range findings {
		if f.Rule.ID == "aws-access-key-id" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 aws-access-key-id findings on one line, got %d", count)
	}
}

func TestContextRedactsAllSecretsOnLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.txt")
	content := "AKIA1234567890ABCDEF postgres://user:s3cretpw99@host/db\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	sc := New(DefaultRules)
	findings, err := sc.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile error: %v", err)
	}
	if len(findings) < 2 {
		t.Fatalf("expected at least 2 findings, got %d", len(findings))
	}
	for _, f := range findings {
		if strings.Contains(f.Context, "AKIA1234567890ABCDEF") {
			t.Errorf("context exposes raw AWS key: %s", f.Context)
		}
		if strings.Contains(f.Context, "s3cretpw99") {
			t.Errorf("context exposes raw db password: %s", f.Context)
		}
	}
}

// ---------------------------------------------------------------------------
// File rules
// ---------------------------------------------------------------------------

func TestFileRuleMatches(t *testing.T) {
	find := func(id string) FileRule {
		for _, fr := range DefaultFileRules {
			if fr.ID == id {
				return fr
			}
		}
		t.Fatalf("file rule %q not found", id)
		return FileRule{}
	}

	cases := []struct {
		ruleID string
		base   string
		want   bool
	}{
		{"ssh-private-key-file", "id_rsa", true},
		{"ssh-private-key-file", "id_ed25519", true},
		{"ssh-private-key-file", "id_rsa.pub", false},
		{"key-material-file", "server.key", true},
		{"key-material-file", "keystore.jks", true},
		{"key-material-file", "cert.p12", true},
		{"pem-file", "cert.pem", true},
		{"dotenv-file", ".env", true},
		{"dotenv-file", ".env.local", true},
		{"dotenv-file", ".env.example", false},
		{"dotenv-file", "config.env", false},
		{"credential-store-file", ".netrc", true},
		{"credential-store-file", ".git-credentials", true},
		{"terraform-state-file", "prod.tfstate", true},
		{"terraform-state-file", "prod.tfstate.backup", true},
		{"kubeconfig-file", "kubeconfig", true},
	}
	for _, tc := range cases {
		t.Run(tc.ruleID+"/"+tc.base, func(t *testing.T) {
			if got := find(tc.ruleID).Matches(tc.base); got != tc.want {
				t.Errorf("FileRule(%s).Matches(%q) = %v, want %v", tc.ruleID, tc.base, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ScanStaged — index content and file rules
// ---------------------------------------------------------------------------

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s (%v)", args, out, err)
	}
}

// ---------------------------------------------------------------------------
// Concurrency — a shared Scanner is safe across goroutines
// ---------------------------------------------------------------------------

// TestScanner_ConcurrentScanNoRace fails under `go test -race` if
// loadIgnoreOnce mutates shared state (s.Rules, s.Allowlist, the once guard)
// without synchronization. The ignore file carries a watch rule and an
// allowlist entry so loading appends to both slices.
func TestScanner_ConcurrentScanNoRace(t *testing.T) {
	dir := t.TempDir()
	ignorePath := filepath.Join(dir, ".suspenders.yaml")
	content := "rules:\n" +
		"  - aws-access-key-id\n" +
		"allowlist:\n" +
		"  - match: known-safe-value\n" +
		"watch:\n" +
		"  - id: custom-watch\n" +
		"    pattern: FOOBAR\n" +
		"    severity: high\n"
	if err := os.WriteFile(ignorePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	sc := New(DefaultRules)
	sc.IgnoreFile = ignorePath

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := sc.ScanLine("f.go", 1, `token = "Xk9Lm2Qp4Rz8Nv3Wb7Yd"`); err != nil {
				t.Errorf("ScanLine: %v", err)
			}
		})
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Skip surfacing — oversize / unreadable files are reported, not silent
// ---------------------------------------------------------------------------

func TestScanDir_oversizeFileSurfacedAsSkip(t *testing.T) {
	dir := t.TempDir()
	// A tracked file larger than maxFileBytes with a secret inside. Its
	// content must not be scanned, and the skip must be surfaced.
	big := "AKIA1234567890ABCDEF\n" + strings.Repeat("a", maxFileBytes+1)
	initGitRepo(t, dir, map[string]string{"big.txt": big})

	var skips []SkippedFile
	sc := New(DefaultRules)
	sc.OnSkip = func(s SkippedFile) { skips = append(skips, s) }

	findings, err := sc.ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir error: %v", err)
	}
	for _, f := range findings {
		if filepath.Base(f.File) == "big.txt" {
			t.Errorf("oversize file content should not be scanned, got finding %+v", f)
		}
	}
	if len(skips) != 1 || filepath.Base(skips[0].Path) != "big.txt" {
		t.Fatalf("expected one skip for big.txt, got %+v", skips)
	}
}

func TestScanStaged_oversizeBlobSurfacedAsSkip(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{"clean.txt": "ok\n"})

	big := "AKIA1234567890ABCDEF\n" + strings.Repeat("a", maxFileBytes+1)
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "big.txt")

	var skips []SkippedFile
	sc := New(DefaultRules)
	sc.OnSkip = func(s SkippedFile) { skips = append(skips, s) }

	findings, err := sc.ScanStaged(dir)
	if err != nil {
		t.Fatalf("ScanStaged error: %v", err)
	}
	for _, f := range findings {
		if filepath.Base(f.File) == "big.txt" {
			t.Errorf("oversize staged blob content should not be scanned, got %+v", f)
		}
	}
	if len(skips) != 1 || filepath.Base(skips[0].Path) != "big.txt" {
		t.Fatalf("expected one skip for big.txt, got %+v", skips)
	}
}

func TestScanStaged(t *testing.T) {
	t.Run("scans index content not working tree", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir, map[string]string{"app.txt": "clean\n"})

		// Stage a secret, then scrub the working tree without re-staging.
		// The secret is still in the index and would be committed.
		path := filepath.Join(dir, "app.txt")
		if err := os.WriteFile(path, []byte("AKIA1234567890ABCDEF\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		gitRun(t, dir, "add", "app.txt")
		if err := os.WriteFile(path, []byte("scrubbed\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanStaged(dir)
		if err != nil {
			t.Fatalf("ScanStaged error: %v", err)
		}
		found := false
		for _, f := range findings {
			if f.Rule.ID == "aws-access-key-id" {
				found = true
			}
		}
		if !found {
			t.Error("expected staged secret to be detected even though working tree is clean")
		}
	})

	t.Run("working tree secret that is not staged is not flagged", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir, map[string]string{"app.txt": "clean\n"})

		path := filepath.Join(dir, "app.txt")
		if err := os.WriteFile(path, []byte("AKIA1234567890ABCDEF\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanStaged(dir)
		if err != nil {
			t.Fatalf("ScanStaged error: %v", err)
		}
		if len(findings) != 0 {
			t.Errorf("expected no findings for unstaged secret, got %+v", findings)
		}
	})

	t.Run("staged sensitive filenames are flagged", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir, map[string]string{"clean.txt": "ok\n"})

		for name, content := range map[string]string{
			".env":         "FOO=bar\n",
			"id_rsa":       "not even a real key\n",
			".env.example": "FOO=\n",
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		gitRun(t, dir, "add", "-f", ".env", "id_rsa", ".env.example")

		sc := New(DefaultRules)
		findings, err := sc.ScanStaged(dir)
		if err != nil {
			t.Fatalf("ScanStaged error: %v", err)
		}
		got := make(map[string]bool)
		for _, f := range findings {
			got[f.Rule.ID+"|"+filepath.Base(f.File)] = true
		}
		if !got["dotenv-file|.env"] {
			t.Error("expected dotenv-file finding for staged .env")
		}
		if !got["ssh-private-key-file|id_rsa"] {
			t.Error("expected ssh-private-key-file finding for staged id_rsa")
		}
		for key := range got {
			if strings.HasSuffix(key, "|.env.example") {
				t.Errorf(".env.example should be excluded, got finding %s", key)
			}
		}
	})
}
