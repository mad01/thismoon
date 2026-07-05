package launchd

import (
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

func TestPlistRoundTrip_ExtraLogs(t *testing.T) {
	def := &service.Definition{
		Name:      "test-service",
		Command:   "/usr/bin/true",
		RunAtLoad: true,
		KeepAlive: true,
		ExtraLogs: map[string]string{
			"sandbox": "/var/log/sandbox-notifications.log",
			"audit":   "/var/log/audit.log",
		},
	}

	data, err := GeneratePlist(def, "test")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	m := &Manager{}
	got := m.plistToDefinition(parsed)

	if len(got.ExtraLogs) != len(def.ExtraLogs) {
		t.Fatalf("ExtraLogs = %v, want %v", got.ExtraLogs, def.ExtraLogs)
	}
	for source, path := range def.ExtraLogs {
		if got.ExtraLogs[source] != path {
			t.Errorf("ExtraLogs[%q] = %q, want %q", source, got.ExtraLogs[source], path)
		}
	}

	// The round-tripped definition must hash identically to the desired one,
	// or reconcile would see a phantom diff on every run
	wantHash, err := def.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	gotHash, err := got.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if wantHash != gotHash {
		t.Errorf("round-tripped hash %s != desired hash %s", gotHash, wantHash)
	}
}

func TestPlistRoundTrip_NoExtraLogs(t *testing.T) {
	def := &service.Definition{
		Name:      "test-service",
		Command:   "/usr/bin/true",
		RunAtLoad: true,
		KeepAlive: true,
	}

	data, err := GeneratePlist(def, "test")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	m := &Manager{}
	got := m.plistToDefinition(parsed)

	wantHash, _ := def.Hash()
	gotHash, err := got.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if wantHash != gotHash {
		t.Errorf(
			"round-tripped hash %s != desired hash %s for a service without extra logs",
			gotHash,
			wantHash,
		)
	}
}
