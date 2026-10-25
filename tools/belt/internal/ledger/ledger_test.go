package ledger

import (
	"maps"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// linked is a loom worktree: its own toplevel and branch, hanging off the
// main checkout.
var linked = gitloc.Location{
	Toplevel:  "/home/.worktrees/thismoon/feat-x",
	Canonical: "/src/thismoon",
	Branch:    "feat/x",
	Repo:      "github.com/mad01/thismoon",
	Linked:    true,
}

// mainCheckout is the canonical checkout itself.
var mainCheckout = gitloc.Location{
	Toplevel:  "/src/thismoon",
	Canonical: "/src/thismoon",
	Branch:    "main",
	Repo:      "github.com/mad01/thismoon",
}

func TestBuild(t *testing.T) {
	tests := []struct {
		name        string
		event       string
		in          Input
		loc         gitloc.Location
		wantTitle   string
		wantMessage string
		wantTags    map[string]string
	}{
		{
			name:  "subagent stop in a linked worktree",
			event: EventSubagentStop,
			in: Input{
				SessionID:            "s1",
				TranscriptPath:       "/t/session.jsonl",
				AgentID:              "agent-def456",
				AgentType:            "Explore",
				AgentTranscriptPath:  "/t/agent-def456.jsonl",
				LastAssistantMessage: "Analysis complete. Found 3 issues.",
				StopHookActive:       true,
			},
			loc:         linked,
			wantTitle:   "subagent stopped (Explore)",
			wantMessage: "Analysis complete. Found 3 issues.",
			wantTags: map[string]string{
				"event":            "subagent-stop",
				"session_id":       "s1",
				"agent_id":         "agent-def456",
				"agent_type":       "Explore",
				"repo":             "github.com/mad01/thismoon",
				"worktree":         "/home/.worktrees/thismoon/feat-x",
				"canonical":        "/src/thismoon",
				"branch":           "feat/x",
				"transcript":       "/t/agent-def456.jsonl",
				"stop_hook_active": "true",
			},
		},
		{
			name:      "subagent start in the main checkout has no worktree tag",
			event:     EventSubagentStart,
			in:        Input{SessionID: "s1", AgentID: "agent-abc123", AgentType: "Explore"},
			loc:       mainCheckout,
			wantTitle: "subagent started (Explore)",
			wantTags: map[string]string{
				"event":      "subagent-start",
				"session_id": "s1",
				"agent_id":   "agent-abc123",
				"agent_type": "Explore",
				"repo":       "github.com/mad01/thismoon",
				"canonical":  "/src/thismoon",
				"branch":     "main",
			},
		},
		{
			name:  "task completed carries the subject as its message",
			event: EventTaskCompleted,
			in: Input{
				SessionID:    "s1",
				TaskID:       "task-001",
				TaskSubject:  "Implement user authentication",
				TeammateName: "implementer",
			},
			wantTitle:   "task completed (implementer)",
			wantMessage: "Implement user authentication",
			wantTags: map[string]string{
				"event":      "task-completed",
				"session_id": "s1",
				"task_id":    "task-001",
				"teammate":   "implementer",
			},
		},
		{
			name:      "pre-compact names its trigger",
			event:     EventPreCompact,
			in:        Input{SessionID: "s1", Trigger: "auto"},
			wantTitle: "compaction starting (auto)",
			wantTags: map[string]string{
				"event":      "pre-compact",
				"session_id": "s1",
				"trigger":    "auto",
			},
		},
		{
			name:  "permission denied names the tool and carries the reason",
			event: EventPermissionDenied,
			in: Input{
				SessionID: "s1",
				ToolName:  "Bash",
				ToolUseID: "toolu_01ABC",
				Reason:    "[Irreversible Local Destruction]",
			},
			wantTitle:   "permission denied (Bash)",
			wantMessage: "[Irreversible Local Destruction]",
			wantTags: map[string]string{
				"event":       "permission-denied",
				"session_id":  "s1",
				"tool":        "Bash",
				"tool_use_id": "toolu_01ABC",
				"reason":      "[Irreversible Local Destruction]",
			},
		},
		{
			name:  "notification names its type and puts the title before the text",
			event: EventNotification,
			in: Input{
				SessionID:           "s1",
				NotificationType:    "permission_prompt",
				NotificationTitle:   "Permission needed",
				NotificationMessage: "Claude needs your permission",
			},
			wantTitle:   "notification (permission_prompt)",
			wantMessage: "Permission needed: Claude needs your permission",
			wantTags: map[string]string{
				"event":             "notification",
				"session_id":        "s1",
				"notification_type": "permission_prompt",
			},
		},
		{
			name:  "notification without a title is the bare text",
			event: EventNotification,
			in: Input{
				NotificationType:    "idle_prompt",
				NotificationMessage: "Claude is waiting for your input",
			},
			wantTitle:   "notification (idle_prompt)",
			wantMessage: "Claude is waiting for your input",
			wantTags: map[string]string{
				"event":             "notification",
				"notification_type": "idle_prompt",
			},
		},
		{
			name:      "an event with no text ignores a stray final message",
			event:     EventTeammateIdle,
			in:        Input{TeammateName: "researcher", LastAssistantMessage: "stray"},
			wantTitle: "teammate idle (researcher)",
			wantTags:  map[string]string{"event": "teammate-idle", "teammate": "researcher"},
		},
		{
			name:      "main-session event omits every empty agent field",
			event:     EventSessionEnd,
			in:        Input{SessionID: "s1", TranscriptPath: "/t/session.jsonl", Reason: "clear"},
			wantTitle: "session ended (clear)",
			wantTags: map[string]string{
				"event":      "session-end",
				"session_id": "s1",
				"reason":     "clear",
				"transcript": "/t/session.jsonl",
			},
		},
		{
			name:      "an event other than stop-failure carries no error tag",
			event:     EventSessionEnd,
			in:        Input{Reason: "clear", Error: "rate_limit"},
			wantTitle: "session ended (clear)",
			wantTags:  map[string]string{"event": "session-end", "reason": "clear"},
		},
		{
			name:      "no qualifier leaves the bare title",
			event:     EventTeammateIdle,
			wantTitle: "teammate idle",
			wantTags:  map[string]string{"event": "teammate-idle"},
		},
		{
			name:      "an event outside the catalog is titled by its name",
			event:     "frobnicate",
			wantTitle: "frobnicate",
			wantTags:  map[string]string{"event": "frobnicate"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(tt.event, tt.in, tt.loc, config.AgentTypes{})
			if got.Source != Source || got.Level != levelInfo {
				t.Errorf(
					"source, level = %q, %q, want %q, %q",
					got.Source,
					got.Level,
					Source,
					levelInfo,
				)
			}
			if got.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMessage)
			}
			if !maps.Equal(got.Tags, tt.wantTags) {
				t.Errorf("tags = %v, want %v", got.Tags, tt.wantTags)
			}
		})
	}
}

// TestBuildStopFailure: a turn that died on an API error is recorded with
// a level that follows its error type, a title naming that type ("unknown"
// when the payload has none or a blank one), and the rendered error with
// its details as the message, with a placeholder when both are empty.
func TestBuildStopFailure(t *testing.T) {
	tests := []struct {
		name        string
		in          Input
		loc         gitloc.Location
		wantLevel   string
		wantTitle   string
		wantMessage string
		wantTags    map[string]string
	}{
		{
			name: "a transient error is warn",
			in: Input{
				SessionID:            "bf16c0dd-5629-4655-a8a0-a3fc5496032b",
				Error:                "overloaded",
				ErrorDetails:         "529 Overloaded",
				LastAssistantMessage: "API Error: Repeated 529 Overloaded errors",
			},
			loc:         mainCheckout,
			wantLevel:   "warn",
			wantTitle:   "turn failed (overloaded)",
			wantMessage: "API Error: Repeated 529 Overloaded errors (529 Overloaded)",
			wantTags: map[string]string{
				"event":      "stop-failure",
				"session_id": "bf16c0dd-5629-4655-a8a0-a3fc5496032b",
				"error":      "overloaded",
				"repo":       "github.com/mad01/thismoon",
				"canonical":  "/src/thismoon",
				"branch":     "main",
			},
		},
		{
			name:        "an error the user must act on is error",
			in:          Input{Error: "billing_error", ErrorDetails: "402 Payment Required"},
			wantLevel:   "error",
			wantTitle:   "turn failed (billing_error)",
			wantMessage: "402 Payment Required",
			wantTags:    map[string]string{"event": "stop-failure", "error": "billing_error"},
		},
		{
			name:        "a missing error type reads as unknown",
			wantLevel:   "error",
			wantTitle:   "turn failed (unknown)",
			wantMessage: "No detail reported.",
			wantTags:    map[string]string{"event": "stop-failure", "error": "unknown"},
		},
		{
			name: "a blank error type reads as unknown and the text is trimmed",
			in: Input{
				Error:                "   ",
				LastAssistantMessage: "  API Error: Rate limit reached\n",
				ErrorDetails:         " ",
			},
			wantLevel:   "error",
			wantTitle:   "turn failed (unknown)",
			wantMessage: "API Error: Rate limit reached",
			wantTags:    map[string]string{"event": "stop-failure", "error": "unknown"},
		},
		{
			name:        "no detail at all gets the placeholder",
			in:          Input{Error: " server_error "},
			wantLevel:   "warn",
			wantTitle:   "turn failed (server_error)",
			wantMessage: "No detail reported.",
			wantTags:    map[string]string{"event": "stop-failure", "error": "server_error"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(EventStopFailure, tt.in, tt.loc, config.AgentTypes{})
			if got.Source != Source || got.Level != tt.wantLevel {
				t.Errorf("source, level = %q, %q, want %q, %q",
					got.Source, got.Level, Source, tt.wantLevel)
			}
			if got.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMessage)
			}
			if !maps.Equal(got.Tags, tt.wantTags) {
				t.Errorf("tags = %v, want %v", got.Tags, tt.wantTags)
			}
		})
	}
}

// TestStopFailureLevelFollowsTheErrorType walks every error type Claude
// Code documents for StopFailure: the API giving up after its retries is
// warn, anything the user has to fix is error.
func TestStopFailureLevelFollowsTheErrorType(t *testing.T) {
	want := map[string]string{
		"rate_limit":             "warn",
		"overloaded":             "warn",
		"server_error":           "warn",
		"cloud_credential_error": "warn",
		"authentication_failed":  "error",
		"oauth_org_not_allowed":  "error",
		"account_on_hold":        "error",
		"billing_error":          "error",
		"invalid_request":        "error",
		"model_not_found":        "error",
		"max_output_tokens":      "error",
		"unknown":                "error",
		"a_type_added_later":     "error",
	}
	for errorType, wantLevel := range want {
		got := Build(
			EventStopFailure,
			Input{Error: errorType},
			gitloc.Location{},
			config.AgentTypes{},
		)
		if got.Level != wantLevel {
			t.Errorf("%s: level = %q, want %q", errorType, got.Level, wantLevel)
		}
	}
}

// TestBuildUnreadablePayload: an entry built from a payload that did not
// decode is still written, at warn level, naming the decode error. warn
// holds even for a failed turn whose error type alone would be error.
func TestBuildUnreadablePayload(t *testing.T) {
	tests := []struct {
		name  string
		event string
		in    Input
	}{
		{"subagent stop", EventSubagentStop, Input{PayloadError: "unexpected EOF"}},
		{
			"stop failure with an error type",
			EventStopFailure,
			Input{Error: "billing_error", PayloadError: "unexpected EOF"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(tt.event, tt.in, gitloc.Location{}, config.AgentTypes{})
			if got.Level != levelWarn {
				t.Errorf("level = %q, want %q", got.Level, levelWarn)
			}
			if want := "hook payload unreadable: unexpected EOF"; got.Message != want {
				t.Errorf("message = %q, want %q", got.Message, want)
			}
			if got.Tags["event"] != tt.event {
				t.Errorf("tags = %v, want the event tag kept", got.Tags)
			}
		})
	}
}

func TestBuildTruncatesTheMessage(t *testing.T) {
	ascii := strings.Repeat("a", maxMessageBytes)
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{"at the cap is kept whole", ascii, ascii},
		{"one byte over is cut to the cap", ascii + "b", ascii},
		{
			// "é" is two bytes; starting it at the last byte the cap allows
			// would split it, so the cut backs up to before it.
			"a rune straddling the cap is dropped whole",
			ascii[:maxMessageBytes-1] + "é and more",
			ascii[:maxMessageBytes-1],
		},
		{
			// "€" is three bytes and ends exactly at the cap.
			"a rune ending at the cap is kept",
			ascii[:maxMessageBytes-3] + "€" + "tail",
			ascii[:maxMessageBytes-3] + "€",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(
				EventSubagentStop,
				Input{LastAssistantMessage: tt.message},
				gitloc.Location{},
				config.AgentTypes{},
			)
			if got.Message != tt.want {
				t.Errorf("message = %d bytes %q..., want %d bytes", len(got.Message),
					got.Message[max(0, len(got.Message)-8):], len(tt.want))
			}
			if !utf8.ValidString(got.Message) {
				t.Errorf("message is not valid UTF-8 after the cut")
			}
		})
	}
}

func TestEventsFollowTheCatalog(t *testing.T) {
	want := []string{
		"subagent-start", "subagent-stop", "task-completed", "teammate-idle",
		"permission-denied", "notification", "pre-compact", "post-compact", "session-end",
		"stop-failure",
	}
	got := Events()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Events() = %v, want %v", got, want)
	}
}

// reviewerTypes is ledger.agent_types as the reviewer definitions declare
// it, keyed by glob because a named spawn reports reviewer-<tier>-<slug>,
// plus an entry whose effort did not resolve.
var reviewerTypes = config.ResolveAgentTypes(map[string]any{
	"reviewer-quick*": map[string]any{"model": "sonnet", "effort": "low"},
	"reviewer-deep*":  map[string]any{"model": "opus", "effort": "high"},
	"half-declared":   map[string]any{"model": "haiku", "effort": "extreme"},
})

// TestTagsCarryDeclaredModelAndEffort: a subagent-start entry whose agent
// type has a ledger.agent_types entry is tagged with its model and effort.
// Any other type, a spawn with no type, and every other event get no such
// tag, and a value that did not resolve drops only its own tag.
func TestTagsCarryDeclaredModelAndEffort(t *testing.T) {
	tests := []struct {
		name       string
		event      string
		agentType  string
		wantModel  string
		wantEffort string
	}{
		{"matching type", EventSubagentStart, "reviewer-deep", "opus", "high"},
		{"another matching type", EventSubagentStart, "reviewer-quick", "sonnet", "low"},
		{
			"a named worker matches its tier's glob", EventSubagentStart,
			"reviewer-deep-csl-prune", "opus", "high",
		},
		{
			"a plain key does not match a longer name", EventSubagentStart,
			"half-declared-x", "", "",
		},
		{"unknown type", EventSubagentStart, "Explore", "", ""},
		{"absent type", EventSubagentStart, "", "", ""},
		{"a dropped effort leaves the model", EventSubagentStart, "half-declared", "haiku", ""},
		{"subagent stop is never tagged", EventSubagentStop, "reviewer-deep", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := Input{SessionID: "s1", AgentID: "a1", AgentType: tt.agentType}
			got := tags(tt.event, in, gitloc.Location{}, reviewerTypes)
			model, hasModel := got["model"]
			effort, hasEffort := got["effort"]
			if model != tt.wantModel || hasModel != (tt.wantModel != "") {
				t.Errorf("model tag = %q (set %v), want %q", model, hasModel, tt.wantModel)
			}
			if effort != tt.wantEffort || hasEffort != (tt.wantEffort != "") {
				t.Errorf("effort tag = %q (set %v), want %q", effort, hasEffort, tt.wantEffort)
			}
		})
	}
}

// TestBuildTagsANamedReviewer is the acceptance case end to end through
// Build: a reviewer spawned under a name carries the model and effort its
// tier's glob declares beside its usual tags.
func TestBuildTagsANamedReviewer(t *testing.T) {
	in := Input{SessionID: "s1", AgentID: "a1", AgentType: "reviewer-deep-csl-prune"}
	got := Build(EventSubagentStart, in, linked, reviewerTypes)
	want := map[string]string{
		"event":      "subagent-start",
		"session_id": "s1",
		"agent_id":   "a1",
		"agent_type": "reviewer-deep-csl-prune",
		"model":      "opus",
		"effort":     "high",
		"repo":       "github.com/mad01/thismoon",
		"worktree":   "/home/.worktrees/thismoon/feat-x",
		"canonical":  "/src/thismoon",
		"branch":     "feat/x",
	}
	if !maps.Equal(got.Tags, want) {
		t.Errorf("tags = %v, want %v", got.Tags, want)
	}
}
