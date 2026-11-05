package vault

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/noteid"
)

// fixtureKey is AWS's documented example access key id: shaped like a real
// one so the scanner fires, and valid nowhere.
const fixtureKey = "AKIAIOSFODNN7EXAMPLE"

func TestWriteRefusesASecret(t *testing.T) {
	tests := []struct {
		name string
		opts WriteOptions
	}{
		{
			"in the body",
			WriteOptions{Tier: TierInbox, Title: "Leak", Body: "key " + fixtureKey + "\n"},
		},
		{"in the frontmatter", WriteOptions{
			Tier: TierInbox, Title: "Leak",
			Set: []markdown.Field{mustField(t, "aws_key", fixtureKey)},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestVault(t, nil)
			_, err := v.Write(tt.opts)
			assertSecretRefused(t, err)
			if _, statErr := os.Stat(filepath.Join(v.Root, "inbox", "leak.md")); !errors.Is(
				statErr,
				os.ErrNotExist,
			) {
				t.Errorf("refused note exists: %v", statErr)
			}
		})
	}
}

func TestEditAndPromoteRefuseASecret(t *testing.T) {
	v := newTestVault(t, map[string]string{
		"inbox/clean.md":  "---\ntitle: Clean\nstatus: raw\n---\nclean body\n",
		"inbox/leaky.md":  "---\ntitle: Leaky\nstatus: raw\n---\nkey " + fixtureKey + "\n",
		"notes/target.md": "---\ntitle: Target\n---\nbody\n",
	})
	clean := filepath.Join(v.Root, "inbox", "clean.md")
	before, _ := os.ReadFile(clean)
	_, err := v.Edit(clean, EditOptions{Append: "and " + fixtureKey})
	assertSecretRefused(t, err)
	if after, _ := os.ReadFile(clean); string(after) != string(before) {
		t.Errorf("refused edit changed the note:\n%s", after)
	}
	_, err = v.Promote(filepath.Join(v.Root, "inbox", "leaky.md"), PromoteOptions{})
	assertSecretRefused(t, err)
	if _, statErr := os.Stat(filepath.Join(v.Root, "notes", "leaky.md")); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Errorf("refused promote wrote its target: %v", statErr)
	}
	_, err = v.Retract(filepath.Join(v.Root, "inbox", "leaky.md"), "error", "", time.Now())
	assertSecretRefused(t, err)
}

func TestSecretSuppression(t *testing.T) {
	tests := []struct {
		name, ignore, body string
	}{
		{"inline marker", "", "key " + fixtureKey + " suspenders:ignore\n"},
		{"vault ignore rule", "rules:\n  - aws-access-key-id\n", "key " + fixtureKey + "\n"},
		{"vault ignore path", "paths:\n  - \"inbox/**\"\n", "key " + fixtureKey + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestVault(t, nil)
			if tt.ignore != "" {
				if err := os.WriteFile(filepath.Join(v.Root, SuspendersFile), []byte(tt.ignore), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := v.Write(WriteOptions{Tier: TierInbox, Title: "Ok", Body: tt.body}); err != nil {
				t.Errorf("Write = %v, want the finding suppressed", err)
			}
		})
	}
}

// TestIdsAndCitationsWriteCleanly pins that note ids, sources[].ref values
// and id@sha citations, which a note carries by design, never read as
// secrets to the entropy rule.
func TestIdsAndCitationsWriteCleanly(t *testing.T) {
	v := newTestVault(t, nil)
	for i := range 100 {
		id := noteid.New(time.Now())
		sha, sha256 := randomHex(t, 20), randomHex(t, 32)
		body := fmt.Sprintf(
			"Builds on %s@%s and id:%s@%s.\nSee [[notes/%s]].\n",
			noteid.New(time.Now()), sha, id, sha[:12], id,
		)
		_, err := v.Write(WriteOptions{
			Tier:  TierNotes,
			Title: fmt.Sprintf("Citing %d", i),
			Body:  body,
			Set: []markdown.Field{
				mustField(t, "id", id),
				mustField(t, "superseded_by", "id:"+noteid.New(time.Now())),
				mustField(t, "sources", fmt.Sprintf(
					"[{ref: 'thismoon:services/brain/internal/vault/vault.go@%s#Vault.Write', "+
						"content_hash: 'sha256:%s'}, {ref: 'brain-notes:notes/%s.md@%s'}]",
					sha, sha256, id, sha,
				)),
			},
		})
		if err != nil {
			t.Fatalf("Write #%d: %v\n%s", i, err, body)
		}
	}
}

func assertSecretRefused(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrSecretFound) {
		t.Fatalf("err = %v, want ErrSecretFound", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "aws-access-key-id (20 characters)") {
		t.Errorf("message %q should name the rule and the match length", msg)
	}
	for _, leak := range []string{fixtureKey, fixtureKey[:4], fixtureKey[len(fixtureKey)-4:]} {
		if strings.Contains(msg, leak) {
			t.Errorf("message %q shows %q of the secret", msg, leak)
		}
	}
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
