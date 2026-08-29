package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/speak/internal/playback"
)

// handlers carries the playback engine shared by all speak tools, plus the
// check list speak_doctor runs.
type handlers struct {
	engine *playback.Engine
	checks func(ctx context.Context) []doctor.Check
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_text",
		Description: "Speak the given text aloud on this machine's speakers using local TTS. " +
			"Returns immediately; audio plays in the background. The text is split into sentences and played in order. " +
			"Keep the returned session id — it is the handle for speak_pause / speak_resume / speak_stop.",
	}, h.handleText)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_file",
		Description: "Read a markdown file aloud on this machine, section by section (a section is the content under each h1/h2). " +
			"Returns immediately; audio plays in the background. Pass `sections` as comma-separated 1-based indices to read only some; omit for all.",
	}, h.handleFile)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_pause",
		Description: "Pause the currently playing audio and release the playback lock so another process can play. Resume with speak_resume. " +
			"Pass `session` to only pause if it matches the active session.",
	}, h.handlePause)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_resume",
		Description: "Resume audio after speak_pause, or restart from the saved position after speak_stop. Re-acquires the playback lock first. " +
			"Pass `session` to only resume if it matches the active session.",
	}, h.handleResume)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_stop",
		Description: "Stop playback. The position is saved, so speak_resume continues from where you stopped. " +
			"Pass `session` to only stop if it matches the active session.",
	}, h.handleStop)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "speak_voices",
		Description: "List the available Kokoro TTS voices that can be passed as `voice` to speak_text / speak_file.",
	}, h.handleVoices)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "speak_status",
		Description: "Report whether the TTS engine is reachable and the current playback state (session, playing/paused/stopped/idle, position, lock holder).",
	}, h.handleStatus)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_doctor",
		Description: "Diagnose speak itself: is the TTS engine reachable, is the playback state directory readable, " +
			"is the optional `speak serve` web surface up and running the installed build. " +
			"Returns one result per check with `ok` false if any failed. " +
			"Read-only — it probes, it changes nothing. " +
			"Call this when another speak tool errors: tts-engine-reachable is the check that gates every tool here, " +
			"while service-reachable and version-skew describe `speak serve`, which playback does not need.",
	}, h.handleDoctor)
}

// ── text ──

type textInput struct {
	Text  string `json:"text"            jsonschema:"the text to speak; may be a sentence or several paragraphs"`
	Voice string `json:"voice,omitempty" jsonschema:"optional Kokoro voice id (af_heart default, af_bella, af_nicole, af_sarah, af_sky, am_adam, am_michael)"`
}

func (h *handlers) handleText(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in textInput,
) (*mcp.CallToolResult, playback.Result, error) {
	return reply(h.engine.SpeakText(in.Text, in.Voice))
}

// ── file ──

type fileInput struct {
	Path     string `json:"path"               jsonschema:"path to a .md file (absolute or ~-prefixed)"`
	Voice    string `json:"voice,omitempty"    jsonschema:"optional Kokoro voice id (see speak_voices)"`
	Sections string `json:"sections,omitempty" jsonschema:"optional comma-separated 1-based section indices to read; empty reads all"`
}

func (h *handlers) handleFile(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in fileInput,
) (*mcp.CallToolResult, playback.Result, error) {
	return reply(h.engine.SpeakFile(in.Path, in.Voice, in.Sections))
}

// ── session-scoped controls ──

type sessionInput struct {
	Session string `json:"session,omitempty" jsonschema:"optional session id from speak_text/speak_file; if set, the call only applies when it matches the active session"`
}

func (h *handlers) handlePause(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in sessionInput,
) (*mcp.CallToolResult, playback.Result, error) {
	return reply(h.engine.Pause(in.Session))
}

func (h *handlers) handleResume(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in sessionInput,
) (*mcp.CallToolResult, playback.Result, error) {
	return reply(h.engine.Resume(in.Session))
}

func (h *handlers) handleStop(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in sessionInput,
) (*mcp.CallToolResult, playback.Result, error) {
	return reply(h.engine.Stop(in.Session))
}

// ── no-arg tools ──

type emptyInput struct{}

func (h *handlers) handleVoices(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ emptyInput,
) (*mcp.CallToolResult, playback.Result, error) {
	return reply(h.engine.Voices())
}

func (h *handlers) handleStatus(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ emptyInput,
) (*mcp.CallToolResult, playback.Result, error) {
	return reply(h.engine.Status())
}

// ── doctor ──

// handleDoctor runs the same checks as `speak doctor` and returns the report
// as structured output, for a client that can call a tool but has no shell.
func (h *handlers) handleDoctor(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ emptyInput,
) (*mcp.CallToolResult, doctor.Report, error) {
	return nil, doctor.Collect(ctx, h.checks(ctx)), nil
}

// reply returns the engine result as both a text content block (the message)
// and structured output, so agents get a readable line and machine-parsable fields.
func reply(res playback.Result) (*mcp.CallToolResult, playback.Result, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: res.Message}},
	}, res, nil
}
