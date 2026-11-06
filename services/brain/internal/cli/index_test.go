package cli

import (
	"errors"
	"strings"
	"testing"
)

// decideSemantic is the pure decision helper used by `brain index` to turn
// an ollama health probe into "skip the semantic pass" vs "fail outright".
// These tests cover the three branches the recipe relies on.

func TestDecideSemantic_HealthyOllama_RunsSemantic(t *testing.T) {
	skip, err := decideSemantic(nil, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if skip {
		t.Error("healthy ollama should not skip semantic")
	}
}

func TestDecideSemantic_HealthyOllama_StrictAlsoRuns(t *testing.T) {
	skip, err := decideSemantic(nil, true)
	if err != nil {
		t.Fatalf("expected no error in strict mode with healthy ollama, got %v", err)
	}
	if skip {
		t.Error("strict mode with healthy ollama should not skip")
	}
}

func TestDecideSemantic_UnreachableOllama_DefaultSkips(t *testing.T) {
	healthErr := errors.New("ollama unreachable at http://localhost:11434")
	skip, err := decideSemantic(healthErr, false)
	if err != nil {
		t.Fatalf("default mode should not return an error, got %v", err)
	}
	if !skip {
		t.Error("unreachable ollama in default mode should skip")
	}
}

func TestDecideSemantic_UnreachableOllama_StrictErrors(t *testing.T) {
	healthErr := errors.New("ollama unreachable at http://localhost:11434")
	skip, err := decideSemantic(healthErr, true)
	if err == nil {
		t.Fatal("strict mode should error when ollama is unreachable")
	}
	if skip {
		t.Error("strict mode should not signal skip when erroring")
	}
	if !strings.Contains(err.Error(), "ollama health") {
		t.Errorf("expected error to mention 'ollama health', got: %v", err)
	}
	if !errors.Is(err, healthErr) {
		t.Error("strict mode error should wrap the underlying health error")
	}
}
