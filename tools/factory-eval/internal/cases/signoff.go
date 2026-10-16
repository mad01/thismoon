package cases

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// signOffKey is the top-level key of the sign-off mapping.
const signOffKey = "sign_off"

// SignOff records who approved the file for paid runs, when, and which
// content they approved: Digest is the file's ContentDigest at sign-off.
type SignOff struct {
	By     string `yaml:"by"     json:"by"`
	Date   string `yaml:"date"   json:"date"`
	Digest string `yaml:"digest" json:"digest"`
}

// CheckSignOff reports every reason the file is not signed off for a paid
// run: a blank name, a date that is not YYYY-MM-DD, or a digest that does
// not match the file as it is now.
func (f *File) CheckSignOff() error {
	var errs []error
	if strings.TrimSpace(f.SignOff.By) == "" {
		errs = append(errs, errors.New("sign_off.by is blank"))
	}
	if _, err := time.Parse(time.DateOnly, f.SignOff.Date); err != nil {
		errs = append(errs, fmt.Errorf("sign_off.date %q is not YYYY-MM-DD", f.SignOff.Date))
	}
	switch f.SignOff.Digest {
	case f.digest:
	case "":
		errs = append(errs, fmt.Errorf("sign_off.digest is empty, want %s", f.digest))
	default:
		errs = append(errs, fmt.Errorf(
			"sign_off.digest %s is not the file's digest %s: the file changed after the sign-off",
			f.SignOff.Digest, f.digest,
		))
	}
	return errors.Join(errs...)
}

// Digest is the file's ContentDigest, the value sign_off.digest must hold.
func (f *File) Digest() string { return f.digest }

// ContentDigest is the lowercase hex sha256 of the cases file with its
// sign-off blanked. Blanked means each line that holds a key of the
// top-level sign_off mapping is rewritten to that key with an empty quoted
// value, at the key's indentation (`  by: ""`), which drops the value and
// any comment on that line. Every other byte stays as it is. The sign_off
// mapping must be a block mapping with each value on its
// key's line; anything else is an error.
func ContentDigest(raw []byte) (string, error) {
	keys, err := signOffKeys(raw)
	if err != nil {
		return "", err
	}
	lines := bytes.SplitAfter(raw, []byte("\n"))
	for _, k := range keys {
		line := lines[k.Line-1]
		blank := strings.Repeat(" ", k.Column-1) + k.Value + `: ""`
		if bytes.HasSuffix(line, []byte("\n")) {
			blank += "\n"
		}
		lines[k.Line-1] = []byte(blank)
	}
	blanked := bytes.Join(lines, nil)
	if err := checkBlanked(blanked); err != nil {
		return "", err
	}
	sum := sha256.Sum256(blanked)
	return hex.EncodeToString(sum[:]), nil
}

// signOffKeys returns the key nodes of the top-level sign_off mapping, or
// none when the file has no sign_off.
func signOffKeys(raw []byte) ([]*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("cases: decode: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("cases: the file is not a mapping")
	}
	top := doc.Content[0].Content
	for i := 0; i+1 < len(top); i += 2 {
		if top[i].Value != signOffKey {
			continue
		}
		m := top[i+1]
		if m.Kind != yaml.MappingNode || m.Style&yaml.FlowStyle != 0 || m.Line == top[i].Line {
			return nil, errors.New("cases: sign_off must be a block mapping, one key per line")
		}
		var keys []*yaml.Node
		for j := 0; j+1 < len(m.Content); j += 2 {
			k, v := m.Content[j], m.Content[j+1]
			if v.Kind != yaml.ScalarNode || v.Line != k.Line ||
				v.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
				return nil, fmt.Errorf(
					"cases: sign_off.%s must be a scalar on its key's line",
					k.Value,
				)
			}
			keys = append(keys, k)
		}
		return keys, nil
	}
	return nil, nil
}

// checkBlanked proves the rewrite blanked the whole sign-off and nothing
// else could have moved: a value that ran onto a second line would leave
// its tail behind and fail to parse here.
func checkBlanked(blanked []byte) error {
	var probe struct {
		SignOff map[string]string `yaml:"sign_off"`
	}
	if err := yaml.Unmarshal(blanked, &probe); err != nil {
		return fmt.Errorf("cases: sign_off values must each fit on their key's line: %w", err)
	}
	for k, v := range probe.SignOff {
		if v != "" {
			return fmt.Errorf("cases: sign_off.%s did not blank", k)
		}
	}
	return nil
}
