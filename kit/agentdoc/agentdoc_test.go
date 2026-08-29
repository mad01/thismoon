package agentdoc

import (
	"errors"
	"strings"
	"testing"
)

var kofFacts = Facts{
	Name:      "keeper-of-facts",
	Bin:       "kof",
	Purpose:   "evidence-pinned assertion store",
	BaseURL:   "http://kof.this",
	StorePath: "~/.local/share/kof",
	LogPath:   "~/.local/share/kof/kof.log",
	HasDoctor: true,
}

func TestRender(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		want    string
		wantErr bool
	}{
		{
			name: "happy path",
			doc:  "Service at {{.BaseURL}}, store in {{.StorePath}}, run {{.Bin}}.",
			want: "Service at http://kof.this, store in ~/.local/share/kof, run kof.",
		},
		{
			name: "no placeholders",
			doc:  "plain text",
			want: "plain text",
		},
		{
			name:    "unknown placeholder",
			doc:     "see {{.BaseURl}}",
			wantErr: true,
		},
		{
			name:    "malformed template",
			doc:     "see {{.BaseURL",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.doc, kofFacts)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Render(%q) = %q, want error", tc.doc, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Render(%q) error: %v", tc.doc, err)
			}
			if got != tc.want {
				t.Errorf("Render(%q) = %q, want %q", tc.doc, got, tc.want)
			}
		})
	}
}

func TestInstructions(t *testing.T) {
	tests := []struct {
		name  string
		facts Facts
		want  string
	}{
		{
			name:  "service with doctor",
			facts: kofFacts,
			want: "kof: evidence-pinned assertion store.\n" +
				"Tools call the local keeper-of-facts service (default http://kof.this) via this stdio shim; the service must be running.\n" +
				"On any tool error or unexpected empty result: run 'kof doctor'. Full doc: 'kof docs'.",
		},
		{
			name: "service without doctor",
			facts: Facts{
				Name: "present", Bin: "present",
				Purpose: "briefing page generator", BaseURL: "http://present.this",
			},
			want: "present: briefing page generator.\n" +
				"Tools call the local present service (default http://present.this) via this stdio shim; the service must be running.\n" +
				"On any tool error or unexpected empty result: see 'present docs'.",
		},
		{
			name: "cli-only with doctor",
			facts: Facts{
				Name: "suspenders", Bin: "suspenders",
				Purpose: "secret-scanning pre-commit guard", HasDoctor: true,
			},
			want: "suspenders: secret-scanning pre-commit guard.\n" +
				"On any tool error or unexpected empty result: run 'suspenders doctor'. Full doc: 'suspenders docs'.",
		},
		{
			name: "cli-only without doctor",
			facts: Facts{
				Name: "toss", Bin: "toss",
				Purpose: "file sharing helper",
			},
			want: "toss: file sharing helper.\n" +
				"On any tool error or unexpected empty result: see 'toss docs'.",
		},
		{
			name: "purpose with trailing period",
			facts: Facts{
				Name: "toss", Bin: "toss",
				Purpose: "file sharing helper.",
			},
			want: "toss: file sharing helper.\n" +
				"On any tool error or unexpected empty result: see 'toss docs'.",
		},
		{
			name: "doctor tool leads the recovery line",
			facts: Facts{
				Name: "keeper-of-facts", Bin: "kof",
				Purpose: "evidence-pinned assertion store", BaseURL: "http://kof.this",
				HasDoctor: true, MCPDoctorTool: "kof_doctor",
			},
			want: "kof: evidence-pinned assertion store.\n" +
				"Tools call the local keeper-of-facts service (default http://kof.this) via this stdio shim; the service must be running.\n" +
				"On any tool error or unexpected empty result: call 'kof_doctor' or run 'kof doctor'. Full doc: 'kof docs'.",
		},
		{
			name: "doctor tool without a doctor subcommand",
			facts: Facts{
				Name: "worklog", Bin: "worklog",
				Purpose: "resumable cross-session work state", MCPDoctorTool: "worklog_doctor",
			},
			want: "worklog: resumable cross-session work state.\n" +
				"On any tool error or unexpected empty result: call 'worklog_doctor'. Full doc: 'worklog docs'.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Instructions(tc.facts)
			if got != tc.want {
				t.Errorf("Instructions() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHint(t *testing.T) {
	if got := Hint(nil, kofFacts); got != nil {
		t.Fatalf("Hint(nil) = %v, want nil", got)
	}

	base := errors.New("connection refused")

	t.Run("with doctor", func(t *testing.T) {
		got := Hint(base, kofFacts)
		if !errors.Is(got, base) {
			t.Errorf("Hint result does not wrap the original error: %v", got)
		}
		want := "connection refused (run 'kof doctor' to diagnose)"
		if got.Error() != want {
			t.Errorf("Hint() = %q, want %q", got.Error(), want)
		}
	})

	t.Run("without doctor", func(t *testing.T) {
		f := Facts{Name: "toss", Bin: "toss"}
		got := Hint(base, f)
		if !errors.Is(got, base) {
			t.Errorf("Hint result does not wrap the original error: %v", got)
		}
		want := "connection refused (see 'toss docs' for troubleshooting)"
		if got.Error() != want {
			t.Errorf("Hint() = %q, want %q", got.Error(), want)
		}
	})

	t.Run("unwrap", func(t *testing.T) {
		got := Hint(base, kofFacts)
		if errors.Unwrap(got) != base {
			t.Errorf("errors.Unwrap(Hint()) = %v, want the original error", errors.Unwrap(got))
		}
	})
}

func TestInstructionsStayThreeLines(t *testing.T) {
	// Ten servers inject this into every session whether or not anything
	// fails, so the block is hard-capped at three lines (ADR-0009).
	facts := []Facts{
		kofFacts,
		{Name: "toss", Bin: "toss", Purpose: "file sharing helper"},
		{Name: "csl", Bin: "csl", Purpose: "local code search", MCPNote: "Nothing must be running."},
		{
			Name: "kof", Bin: "kof", Purpose: "assertion store", BaseURL: "http://kof.this",
			HasDoctor: true, MCPDoctorTool: "kof_doctor",
		},
	}
	for _, f := range facts {
		got := Instructions(f)
		if n := len(strings.Split(got, "\n")); n > 3 {
			t.Errorf("Instructions(%s) has %d lines, want at most 3:\n%s", f.Bin, n, got)
		}
	}
}

func TestRegistrationSnippet(t *testing.T) {
	got := RegistrationSnippet(kofFacts)
	for _, want := range []string{
		"claude mcp add kof -- kof mcp",
		"[mcp_servers.kof]",
		`command = "kof"`,
		`args = ["mcp"]`,
		"~/.codex/config.toml",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RegistrationSnippet() missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "keeper-of-facts") {
		t.Errorf("RegistrationSnippet() names the component, want the binary:\n%s", got)
	}
}

func TestOutputIsASCII(t *testing.T) {
	outputs := []string{
		Instructions(kofFacts),
		Hint(errors.New("boom"), kofFacts).Error(),
		RegistrationSnippet(kofFacts),
	}
	for _, s := range outputs {
		for _, r := range s {
			if r > 127 {
				t.Errorf("non-ASCII rune %q in output %q", r, s)
			}
		}
	}
	if strings.Contains(strings.Join(outputs, ""), "—") {
		t.Error("em dash in output")
	}
}
