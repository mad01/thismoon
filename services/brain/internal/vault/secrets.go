package vault

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/kit/secretscan"
)

// SuspendersFile is the ignore config a vault may keep at its root. brain
// reads it the way the suspenders pre-commit hook reads a repo's, so a value
// the hook lets through, brain lets through too.
const SuspendersFile = ".suspenders.yaml"

// maxReportedSecrets caps the findings a refused write lists.
const maxReportedSecrets = 3

// Secrets scans content bound for the note at the vault-relative path rel
// with the suspenders rules. It honors the vault's .suspenders.yaml and the
// inline suspenders:ignore marker. Each finding's Match is redacted.
func (v *Vault) Secrets(rel string, content []byte) ([]secretscan.Finding, error) {
	s := secretscan.New(secretscan.DefaultRules)
	s.IgnoreFile = filepath.Join(v.Root, SuspendersFile)
	return s.ScanContent(rel, content)
}

// refuseSecrets fails with ErrSecretFound when content bound for path holds
// a secret, naming each rule with the value redacted. It runs before a
// write touches disk, so a refused note is never written.
func (v *Vault) refuseSecrets(path string, content []byte) error {
	rel := v.Rel(path)
	findings, err := v.Secrets(rel, content)
	if err != nil {
		return fmt.Errorf("scan %s for secrets: %w", rel, err)
	}
	if len(findings) == 0 {
		return nil
	}
	parts := make([]string, 0, maxReportedSecrets+1)
	for _, f := range findings[:min(len(findings), maxReportedSecrets)] {
		parts = append(parts, fmt.Sprintf("line %d: %s %s", f.Line, f.Rule.ID, f.Match))
	}
	if extra := len(findings) - maxReportedSecrets; extra > 0 {
		parts = append(parts, fmt.Sprintf("and %d more", extra))
	}
	return fmt.Errorf(
		"%w in %s: %s; nothing was written (remove the value, or mark a false positive "+
			"with suspenders:ignore on its line or in the vault's %s)",
		ErrSecretFound, rel, strings.Join(parts, "; "), SuspendersFile,
	)
}
