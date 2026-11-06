package vault

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/mad01/thismoon/kit/secretscan"
)

// SuspendersFile is the ignore config a vault may keep at its root. brain
// reads it the way the suspenders pre-commit hook reads a repo's, so a value
// the hook lets through, brain lets through too.
const SuspendersFile = ".suspenders.yaml"

// maxReportedSecrets caps the findings a refused write lists.
const maxReportedSecrets = 3

// NulByteRule is the rule id of the finding Secrets reports for a NUL byte.
const NulByteRule = "nul-byte"

// nulRule names the finding Secrets reports for a NUL byte. The scan skips
// content that holds one as binary, so a NUL could hide a secret after it.
var nulRule = secretscan.Rule{
	ID:          NulByteRule,
	Description: "a NUL byte: a note is text, and the secret scan skips a file holding one",
	Severity:    "high",
}

// Secrets scans content bound for the note at the vault-relative path rel
// with the suspenders rules. It honors the vault's .suspenders.yaml and the
// inline suspenders:ignore marker. Each finding's Match is redacted. A NUL
// byte is a finding of its own (nulRule), since the scan treats content
// with one as binary and reads no further.
func (v *Vault) Secrets(rel string, content []byte) ([]secretscan.Finding, error) {
	s := secretscan.New(secretscan.DefaultRules)
	s.IgnoreFile = filepath.Join(v.Root, SuspendersFile)
	found, err := s.ScanContent(rel, content)
	if err != nil {
		return nil, err
	}
	if line, col, ok := nulAt(content); ok {
		nul := secretscan.Finding{
			File: rel, Line: line, Column: col, Rule: nulRule,
			Match: "the note holds a NUL byte, which hides what follows from the secret scan; a note is text",
		}
		found = append([]secretscan.Finding{nul}, found...)
	}
	return found, nil
}

// nulAt returns the 1-based line and column of the first NUL byte in
// content, and false when there is none.
func nulAt(content []byte) (line, col int, ok bool) {
	i := bytes.IndexByte(content, 0)
	if i < 0 {
		return 0, 0, false
	}
	return bytes.Count(content[:i], []byte("\n")) + 1, i - bytes.LastIndexByte(content[:i], '\n'), true
}

// refuseContent refuses content bound for path that is not a text note
// free of secrets: a NUL byte fails with ErrInvalidContent, a secret with
// ErrSecretFound. It runs before a write touches disk, so a refused note is
// never written.
func (v *Vault) refuseContent(path string, content []byte) error {
	if line, col, ok := nulAt(content); ok {
		return fmt.Errorf(
			"%w: %s holds a NUL byte at line %d, column %d; a note is text, and nothing was written",
			ErrInvalidContent, v.Rel(path), line, col,
		)
	}
	return v.refuseSecrets(path, content)
}

// refuseSecrets fails with ErrSecretFound when content bound for path holds
// a secret, naming each finding by its rule, line and length alone: no
// character of the value, since the message lands in terminals, logs and
// agent context. It runs before a write touches disk, so a refused note is
// never written.
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
		parts = append(parts, fmt.Sprintf("line %d: %s (%d characters)",
			f.Line, f.Rule.ID, utf8.RuneCountInString(f.RawMatch)))
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
