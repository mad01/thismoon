package check

import (
	"slices"
	"strings"
	"testing"
)

// fixtureKey is AWS's documented example access key id: a secret-shaped
// value that is not a credential.
const fixtureKey = "AKIAIOSFODNN7EXAMPLE"

func TestRunFindsSecrets(t *testing.T) {
	r, got := runCheck(t, map[string]string{
		"notes/a.md":       note(validated(idA), "The key is "+fixtureKey+".\n"),
		"notes/ignored.md": note(validated(idB), "The key is "+fixtureKey+". suspenders:ignore\n"),
	})
	want := []string{"notes/a.md:7 secret error"}
	if !slices.Equal(got, want) {
		t.Fatalf("findings = %q, want %q", got, want)
	}
	// The message names the rule and the length, as a refused write does,
	// and no character of the value: it lands in CI logs and annotations.
	msg := r.Findings[0].Message
	if msg != "aws-access-key-id (20 characters)" {
		t.Errorf("message = %q, want the rule id and the match length", msg)
	}
	if strings.Contains(msg, fixtureKey[:4]) || strings.Contains(msg, fixtureKey[len(fixtureKey)-4:]) {
		t.Errorf("message %q shows characters of the secret", msg)
	}
}
