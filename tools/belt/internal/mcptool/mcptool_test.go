package mcptool

import "testing"

func TestSplit(t *testing.T) {
	tests := []struct {
		tool       string
		wantServer string
		wantOp     string
		wantOK     bool
	}{
		{"mcp__gh_com__create_pull_request", "gh_com", "create_pull_request", true},
		{"mcp__a__b__c", "a__b", "c", true},
		{"mcp__broken", "", "", false},
		{"Bash", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		server, op, ok := Split(tt.tool)
		if server != tt.wantServer || op != tt.wantOp || ok != tt.wantOK {
			t.Errorf("Split(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.tool, server, op, ok, tt.wantServer, tt.wantOp, tt.wantOK)
		}
	}
}

func TestPublishes(t *testing.T) {
	tests := []struct {
		tool string
		want bool
	}{
		{"mcp__github__add_issue_comment", true},
		{"mcp__github__create_pull_request", true},
		{"mcp__github__push_files", true},
		{"mcp__github__create_branch", true},
		{"mcp__github__issue_write", true},
		// Reads reach belt whenever the matcher names a whole server.
		{"mcp__github__get_file_contents", false},
		{"mcp__github__list_issues", false},
		{"mcp__github__search_code", false},
		{"mcp__github__issue_read", false},
		{"mcp__github__pull_request_read", false},
		// Non-MCP tools are matcher mistakes, not external writes.
		{"Bash", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := Publishes(tt.tool); got != tt.want {
			t.Errorf("Publishes(%q) = %v, want %v", tt.tool, got, tt.want)
		}
	}
}
