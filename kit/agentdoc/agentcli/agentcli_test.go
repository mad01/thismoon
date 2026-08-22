package agentcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/doctor"
)

var kofFacts = agentdoc.Facts{
	Name: "keeper-of-facts", Bin: "kof",
	Purpose: "evidence-pinned assertion store",
	BaseURL: "http://kof.this", HasDoctor: true,
}

func TestDocsCommand(t *testing.T) {
	cmd := DocsCommand("Service at {{.BaseURL}}, run {{.Bin}}.\n", kofFacts)
	if got := cmd.Use; got != "docs" {
		t.Errorf("Use = %q, want %q", got, "docs")
	}
	if !strings.Contains(cmd.Short, "kof") {
		t.Errorf("Short = %q, want it to name the binary", cmd.Short)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "Service at http://kof.this, run kof.\n"
	if got := out.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestDocsCommandBadTemplate(t *testing.T) {
	cmd := DocsCommand("see {{.NoSuchField}}", kofFacts)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil {
		t.Error("Execute() = nil, want a render error for an unknown placeholder")
	}
}

func TestDoctorCommand(t *testing.T) {
	built := false
	cmd := DoctorCommand(kofFacts, func(context.Context) []doctor.Check {
		built = true
		return []doctor.Check{
			{Name: "alpha", Run: func(context.Context) error { return nil }},
		}
	})
	if got := cmd.Use; got != "doctor" {
		t.Errorf("Use = %q, want %q", got, "doctor")
	}
	if built {
		t.Fatal("checks built at construction time, want lazily at run time")
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !built {
		t.Error("checks func never invoked")
	}
	want := "ok alpha\nall checks passed; for semantics and gotchas run 'kof docs'\n"
	if got := out.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestDoctorCommandFailure(t *testing.T) {
	cmd := DoctorCommand(kofFacts, func(context.Context) []doctor.Check {
		return []doctor.Check{
			{Name: "alpha", Run: func(context.Context) error { return errors.New("boom") }},
		}
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "1 of 1 checks failed") {
		t.Errorf("Execute() error = %v, want the failed-checks count", err)
	}
	if got := out.String(); got != "FAIL alpha: boom\n" {
		t.Errorf("output = %q, want %q", got, "FAIL alpha: boom\n")
	}
}
