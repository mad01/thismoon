package service

import (
	"testing"
)

func TestDefinitionValidate_ExtraLogs(t *testing.T) {
	tests := []struct {
		name      string
		extraLogs map[string]string
		wantErr   bool
	}{
		{name: "no extra logs is fine", extraLogs: nil, wantErr: false},
		{
			name:      "valid extra log",
			extraLogs: map[string]string{"sandbox": "/var/log/sandbox.log"},
			wantErr:   false,
		},
		{
			name: "multiple valid extra logs",
			extraLogs: map[string]string{
				"sandbox":     "/var/log/a.log",
				"audit.trail": "/var/log/b.log",
			},
			wantErr: false,
		},
		{
			name:      "relative path rejected",
			extraLogs: map[string]string{"sandbox": "logs/sandbox.log"},
			wantErr:   true,
		},
		{
			name:      "invalid source name rejected",
			extraLogs: map[string]string{"bad name!": "/var/log/a.log"},
			wantErr:   true,
		},
		{
			name:      "reserved name stdout rejected",
			extraLogs: map[string]string{"stdout": "/var/log/a.log"},
			wantErr:   true,
		},
		{
			name:      "reserved name stderr rejected",
			extraLogs: map[string]string{"stderr": "/var/log/a.log"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition{
				Name:      "test-service",
				Command:   "/usr/bin/true",
				ExtraLogs: tt.extraLogs,
			}
			err := def.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefinitionHash_ExtraLogs(t *testing.T) {
	base := Definition{
		Name:    "test-service",
		Command: "/usr/bin/echo",
	}

	withLogs := base
	withLogs.ExtraLogs = map[string]string{"sandbox": "/var/log/sandbox.log"}

	h1, err := base.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	h2, err := withLogs.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if h1 == h2 {
		t.Error("expected different hashes when extra logs differ")
	}

	// nil and empty map must hash identically — otherwise every service
	// predating extra logs would reconcile on the next add
	withEmpty := base
	withEmpty.ExtraLogs = map[string]string{}
	h3, err := withEmpty.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if h1 != h3 {
		t.Error("expected identical hashes for nil and empty extra logs")
	}
}
