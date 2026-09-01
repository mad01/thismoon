package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/reminder/internal/client"
)

// handlers carries the dependencies shared by all reminder tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views reminders in a browser
	checks func(ctx context.Context) []doctor.Check
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "reminder_create",
		Description: "Create a reminder that fires a macOS notification at its due time. " +
			"Set the time ONE of two ways: `due` as an absolute RFC3339 timestamp (you know today's date; convert natural language like 'tomorrow 9am' or 'next monday' to RFC3339 yourself, in the user's local timezone), OR `in` as a Go duration ('2h30m', '45m', '90s') for a relative offset from now. " +
			"Optional `repeat` makes it recurring: 'daily', 'weekly', or a Go duration like '24h'; omit for a one-shot. " +
			"Keep the returned id: it is the handle for reminder_get / reminder_edit / reminder_cancel.",
	}, h.handleCreate)

	mcp.AddTool(s, &mcp.Tool{
		Name: "reminder_list",
		Description: "List reminders (id, title, due time, repeat, status, and an `overdue` flag), soonest due first. " +
			"Optional `status` filter: 'pending' (armed), 'fired' (a one-shot that already fired), 'done', or 'cancelled'.",
	}, h.handleList)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "reminder_get",
		Description: "Get one reminder's full detail by id.",
	}, h.handleGet)

	mcp.AddTool(s, &mcp.Tool{
		Name: "reminder_edit",
		Description: "Edit a reminder by id (the `id` is REQUIRED). Change `title`, `body`, `due` (absolute RFC3339), and/or `repeat`; only the fields you pass change. " +
			"Moving `due` to a future time re-arms a reminder that already fired.",
	}, h.handleEdit)

	mcp.AddTool(s, &mcp.Tool{
		Name: "reminder_cancel",
		Description: "Cancel a reminder by id, a soft stop that keeps the record (status becomes 'cancelled') and prevents it from firing. " +
			"To remove a reminder permanently, delete it from the web page.",
	}, h.handleCancel)

	mcp.AddTool(s, &mcp.Tool{
		Name: "reminder_test",
		Description: "Fire a notification NOW to verify macOS notifications actually work on this machine. Use this to confirm the notification path before trusting a real reminder. " +
			"It is a dry run: it doesn't change any reminder's state (no one-shot consumed, no recurring schedule advanced) and isn't recorded in the event log. " +
			"Pass `id` to send that reminder's exact notification (same title/body it would show when due), or omit `id` for a generic 'notifications are working' test (good right after setup).",
	}, h.handleTest)

	mcp.AddTool(s, &mcp.Tool{
		Name: "reminder_fire",
		Description: "Fire a reminder for REAL right now (the `id` is REQUIRED), exactly as if it had just come due: it sends the notification, records the event, and advances state. A recurring reminder reschedules to its next occurrence, a one-shot becomes 'fired'. " +
			"This isn't a dry run; use reminder_test to only verify notifications without changing state.",
	}, h.handleFire)

	mcp.AddTool(s, &mcp.Tool{
		Name: "reminder_doctor",
		Description: "Diagnose reminder itself: is serve reachable, is the store readable, is the running build " +
			"the installed one. Returns one result per check with `ok` false if any failed. " +
			"Read-only: it probes, it changes nothing, and it sends no notification. " +
			"Call this when another reminder tool errors; use `reminder_test` instead to check that macOS " +
			"notifications actually reach the user.",
	}, h.handleDoctor)
}

// out is the tool response shape: the reminder plus the web URL to view it.
type out struct {
	Reminder client.Reminder `json:"reminder"`
	URL      string          `json:"url"      jsonschema:"web page where the user can view and manage reminders"`
}

func (h *handlers) one(r client.Reminder) out { return out{Reminder: r, URL: h.webURL} }

// ── create ──

type createInput struct {
	Title  string `json:"title"            jsonschema:"what to be reminded about"`
	Body   string `json:"body,omitempty"   jsonschema:"optional longer note shown in the notification body"`
	Due    string `json:"due,omitempty"    jsonschema:"absolute due time as RFC3339 (e.g. 2026-06-26T09:00:00+02:00). Provide this OR 'in', not both."`
	In     string `json:"in,omitempty"     jsonschema:"relative due time as a Go duration from now (e.g. '2h30m', '45m'). Provide this OR 'due', not both."`
	Repeat string `json:"repeat,omitempty" jsonschema:"recurrence: 'daily', 'weekly', or a Go duration like '24h'. Omit for a one-shot."`
}

func (h *handlers) handleCreate(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in createInput,
) (*mcp.CallToolResult, out, error) {
	r, err := h.client.Create(
		client.CreateBody{
			Title:  in.Title,
			Body:   in.Body,
			Due:    in.Due,
			In:     in.In,
			Repeat: in.Repeat,
		},
	)
	if err != nil {
		return nil, out{}, err
	}
	return nil, h.one(r), nil
}

// ── list ──

type listInput struct {
	Status string `json:"status,omitempty" jsonschema:"optional filter: pending | fired | done | cancelled"`
}

type listOutput struct {
	Reminders []client.Reminder `json:"reminders"`
	URL       string            `json:"url"`
}

func (h *handlers) handleList(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in listInput,
) (*mcp.CallToolResult, listOutput, error) {
	rs, err := h.client.List(in.Status)
	if err != nil {
		return nil, listOutput{}, err
	}
	return nil, listOutput{Reminders: rs, URL: h.webURL}, nil
}

// ── get ──

type idInput struct {
	ID string `json:"id" jsonschema:"reminder id returned by reminder_create"`
}

func (h *handlers) handleGet(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in idInput,
) (*mcp.CallToolResult, out, error) {
	r, err := h.client.Get(in.ID)
	if err != nil {
		return nil, out{}, err
	}
	return nil, h.one(r), nil
}

// ── edit ──

type editInput struct {
	ID     string  `json:"id"               jsonschema:"reminder id to edit (required)"`
	Title  *string `json:"title,omitempty"  jsonschema:"new title; omit to leave unchanged"`
	Body   *string `json:"body,omitempty"   jsonschema:"new body note; omit to leave unchanged"`
	Due    *string `json:"due,omitempty"    jsonschema:"new absolute due time as RFC3339; omit to leave unchanged. A future time re-arms a fired reminder."`
	Repeat *string `json:"repeat,omitempty" jsonschema:"new recurrence ('daily'|'weekly'|Go duration|empty for one-shot); omit to leave unchanged"`
}

func (h *handlers) handleEdit(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in editInput,
) (*mcp.CallToolResult, out, error) {
	r, err := h.client.Update(
		in.ID,
		client.UpdateBody{Title: in.Title, Body: in.Body, Due: in.Due, Repeat: in.Repeat},
	)
	if err != nil {
		return nil, out{}, err
	}
	return nil, h.one(r), nil
}

// ── cancel ──

func (h *handlers) handleCancel(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in idInput,
) (*mcp.CallToolResult, out, error) {
	r, err := h.client.Cancel(in.ID)
	if err != nil {
		return nil, out{}, err
	}
	return nil, h.one(r), nil
}

// ── test ──

type testInput struct {
	ID string `json:"id,omitempty" jsonschema:"reminder id to send a test notification for; omit for a generic 'notifications are working' test"`
}

// testOutput carries an ok flag, the web URL, and — for an id-scoped test — the
// (unchanged) reminder. Reminder is nil for a global test.
type testOutput struct {
	OK       bool             `json:"ok"`
	Reminder *client.Reminder `json:"reminder,omitempty"`
	URL      string           `json:"url"`
}

func (h *handlers) handleTest(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in testInput,
) (*mcp.CallToolResult, testOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		if err := h.client.TestNotification(); err != nil {
			return nil, testOutput{}, err
		}
		return nil, testOutput{OK: true, URL: h.webURL}, nil
	}
	r, err := h.client.Test(in.ID)
	if err != nil {
		return nil, testOutput{}, err
	}
	return nil, testOutput{OK: true, Reminder: &r, URL: h.webURL}, nil
}

// ── fire ──

func (h *handlers) handleFire(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in idInput,
) (*mcp.CallToolResult, out, error) {
	r, err := h.client.Fire(in.ID)
	if err != nil {
		return nil, out{}, err
	}
	return nil, h.one(r), nil
}

// ── doctor ──

type doctorInput struct{}

// handleDoctor runs the same checks as `reminder doctor` and returns the
// report. A failing check is a result, not a tool error: the caller asked
// what is wrong, and an error would hide the answer behind a transport
// failure.
func (h *handlers) handleDoctor(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ doctorInput,
) (*mcp.CallToolResult, doctor.Report, error) {
	return nil, doctor.Collect(ctx, h.checks(ctx)), nil
}
