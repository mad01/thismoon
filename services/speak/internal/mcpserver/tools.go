package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/speak/internal/playback"
	"github.com/mad01/thismoon/services/speak/internal/provider"
)

// handlers carries the playback engine and provider shared by all speak
// tools, plus the check list speak_doctor runs.
type handlers struct {
	engine   *playback.Engine
	provider *provider.Provider
	checks   func(ctx context.Context) []doctor.Check
}

func registerTools(s *mcp.Server, h *handlers) error {
	voices := h.provider.Voices()
	textSchema, err := withVoices[textInput](voices)
	if err != nil {
		return err
	}
	fileSchema, err := withVoices[fileInput](voices)
	if err != nil {
		return err
	}
	// Text goes to a third party when the provider is remote, which is what
	// the open-world hint tells a client.
	remote := h.provider.Config().Remote

	mcp.AddTool(s, &mcp.Tool{
		Name:        "speak_text",
		InputSchema: textSchema,
		Description: "Speak the given text aloud on this machine's speakers through the configured TTS provider. " +
			"Returns once the first sentence is synthesized; the rest plays in the background. The text is split into sentences and played in order. " +
			"If the TTS backend cannot synthesize, the call fails with an UNAVAILABLE reply naming the reason and nothing plays. " +
			"Keep the returned session id: it is the handle for speak_pause / speak_resume / speak_stop.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(true),
			IdempotentHint:  false,
			OpenWorldHint:   new(remote),
		},
	}, h.handleText)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "speak_file",
		InputSchema: fileSchema,
		Description: "Read a markdown file aloud on this machine, section by section (a section is the content under each h1/h2). " +
			"Returns once the first sentence is synthesized; the rest plays in the background. Pass `sections` as comma-separated 1-based indices to read only some; omit for all. " +
			"If the TTS backend cannot synthesize, the call fails with an UNAVAILABLE reply naming the reason and nothing plays.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(true),
			IdempotentHint:  false,
			OpenWorldHint:   new(remote),
		},
	}, h.handleFile)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_pause",
		Description: "Pause the currently playing audio and release the playback lock so another process can play. Resume with speak_resume. " +
			"Pass `session` to only pause if it matches the active session.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(false),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, h.handlePause)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_resume",
		Description: "Resume audio after speak_pause, or restart from the saved position after speak_stop. Re-acquires the playback lock first. " +
			"Pass `session` to only resume if it matches the active session.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(false),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, h.handleResume)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_stop",
		Description: "Stop playback. The position is saved, so speak_resume continues from where you stopped. " +
			"Pass `session` to only stop if it matches the active session.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, h.handleStop)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_voices",
		Description: "List the voices the configured TTS provider offers, with the default marked, that can be passed as `voice` to speak_text / speak_file. " +
			"Also says where the list came from: the config's curated list, discovered from the provider, or speak's built-in catalog.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleVoices)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "speak_status",
		Description: "Report whether the TTS engine is reachable, the TTS health recorded from recent syntheses (ok, degraded or down, with the reason), and the current playback state (session, playing/paused/stopped/idle, position, lock holder).",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleStatus)

	mcp.AddTool(s, &mcp.Tool{
		Name: "speak_doctor",
		Description: "Diagnose speak itself: does the config load, is each configured TTS provider usable, " +
			"can the active one synthesize, is the playback state directory readable, " +
			"is the optional `speak serve` web surface up and running the installed build. " +
			"Returns one result per check with `ok` false if any failed. " +
			"Read-only apart from one short test synthesis through the active provider. " +
			"Call this when another speak tool errors: config, the active provider check and tts-synthesis gate every tool here " +
			"(tts-synthesis speaks a short test phrase, so it catches a provider that answers but cannot synthesize), " +
			"while service-reachable and version-skew describe `speak serve`, which playback doesn't need.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(remote),
		},
	}, h.handleDoctor)
	return nil
}

// ── text ──

type textInput struct {
	Text  string `json:"text"            jsonschema:"the text to speak; may be a sentence or several paragraphs"`
	Voice string `json:"voice,omitempty" jsonschema:"optional voice id; see speak_voices"`
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
	Voice    string `json:"voice,omitempty"    jsonschema:"optional voice id; see speak_voices"`
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
	return reply(playback.Result{Message: describeVoices(h.provider)})
}

// describeVoices renders speak_voices' reply: the provider, where the list
// came from, and the voices with the default marked.
func describeVoices(p *provider.Provider) string {
	v := p.Voices()
	var b strings.Builder
	fmt.Fprintf(
		&b,
		"provider: %s (%s)\ndefault: %s\nsource: %s\n",
		p.Name(),
		p.Model(),
		v.Default,
		v.Source,
	)
	if v.Note != "" {
		fmt.Fprintf(&b, "note: %s\n", v.Note)
	}
	if len(v.List) == 0 {
		b.WriteString("voices: not listed; any voice id the provider accepts can be passed\n")
		return b.String()
	}
	b.WriteString("voices:\n")
	for _, name := range v.List {
		if name == v.Default {
			fmt.Fprintf(&b, "  %s (default)\n", name)
			continue
		}
		fmt.Fprintf(&b, "  %s\n", name)
	}
	return b.String()
}

// withVoices infers In's input schema and pins its voice property to the
// provider's voices, so a client offers exactly those and the server
// rejects anything else. With no known list the property stays free text.
func withVoices[In any](v provider.Voices) (*jsonschema.Schema, error) {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		return nil, fmt.Errorf("mcpserver: infer input schema: %w", err)
	}
	prop, ok := schema.Properties["voice"]
	if !ok {
		return nil, fmt.Errorf("mcpserver: input schema has no voice property")
	}
	if len(v.List) == 0 {
		prop.Description = fmt.Sprintf(
			"optional voice id; default %s. speak_voices shows what is known",
			v.Default,
		)
		return schema, nil
	}
	prop.Enum = make([]any, len(v.List))
	for i, name := range v.List {
		prop.Enum[i] = name
	}
	prop.Description = fmt.Sprintf(
		"optional voice id; default %s. speak_voices describes the list",
		v.Default,
	)
	return schema, nil
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
// and structured output, so agents get a readable line and machine-parsable
// fields. A failed result is flagged as a tool error, so a client treats
// "speech could not start" as a failure rather than a reply to read past.
func reply(res playback.Result) (*mcp.CallToolResult, playback.Result, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: res.Message}},
		IsError: res.Failed,
	}, res, nil
}
