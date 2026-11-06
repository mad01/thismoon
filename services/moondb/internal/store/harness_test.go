package store

import "testing"

func TestSettleHarness(t *testing.T) {
	tests := []struct {
		cur, next, want string
	}{
		{"", HarnessPi, HarnessPi},
		{"", "", ""},
		{HarnessClaudeCode, HarnessPi, HarnessPi},
		{HarnessPi, HarnessClaudeCode, HarnessPi},
		{HarnessClaudeCode, "", HarnessClaudeCode},
		{HarnessPi, HarnessPi, HarnessPi},
	}
	for _, tt := range tests {
		if got := SettleHarness(tt.cur, tt.next); got != tt.want {
			t.Errorf("SettleHarness(%q, %q) = %q; want %q", tt.cur, tt.next, got, tt.want)
		}
	}
}

func TestSessionHarness(t *testing.T) {
	tests := []struct {
		harness, entrypoint, want string
	}{
		{HarnessClaudeCode, "pi-print", HarnessPi},
		{HarnessClaudeCode, "pi-tui", HarnessPi},
		{HarnessClaudeCode, "cli", HarnessClaudeCode},
		{HarnessClaudeCode, "", HarnessClaudeCode},
		{HarnessPi, "pi-rpc", HarnessPi},
	}
	for _, tt := range tests {
		s := Session{Harness: tt.harness, Entrypoint: tt.entrypoint}
		if got := SessionHarness(s); got != tt.want {
			t.Errorf("SessionHarness(%q, entrypoint %q) = %q; want %q",
				tt.harness, tt.entrypoint, got, tt.want)
		}
	}
}
