package main

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/mad01/thismoon/services/csl/internal/ask"
)

// promptHash ties a run to the worker instructions that produced it. The
// instructions are csl_ask's own (ask.SystemPrompt), so a run measures the
// wording the tool sends, and run.json records this hash.
func promptHash() string {
	sum := sha256.Sum256([]byte(ask.SystemPrompt))
	return hex.EncodeToString(sum[:8])
}
