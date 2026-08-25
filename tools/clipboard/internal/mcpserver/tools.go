package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/clipboard/internal/clip"
)

func registerTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "clipboard_copy",
		Description: "Write text to the macOS system clipboard (pbcopy), replacing its current contents. " +
			"Use when the user says copy this, put it on my clipboard, copy to clipboard, " +
			"or wants a snippet, command, or link ready to paste somewhere else. " +
			"The text lands verbatim: include or omit a trailing newline deliberately.",
	}, handleCopy)

	mcp.AddTool(s, &mcp.Tool{
		Name: "clipboard_paste",
		Description: "Read the current macOS system clipboard contents (pbpaste). " +
			"Use when the user says paste from my clipboard, what's on my clipboard, " +
			"or asks to work with something they just copied. " +
			"Text only: an image or file on the clipboard comes back empty, with a note saying so.",
	}, handlePaste)
}

type copyInput struct {
	Text string `json:"text" jsonschema:"the exact text to place on the clipboard, copied verbatim"`
}

type copyOutput struct {
	CopiedBytes int `json:"copied_bytes" jsonschema:"length of the text now on the clipboard, for confirmation"`
}

func handleCopy(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in copyInput,
) (*mcp.CallToolResult, copyOutput, error) {
	if in.Text == "" {
		return nil, copyOutput{}, fmt.Errorf("nothing to copy: text is empty")
	}
	if err := clip.New().Copy(in.Text); err != nil {
		return nil, copyOutput{}, err
	}
	return nil, copyOutput{CopiedBytes: len(in.Text)}, nil
}

type pasteInput struct{}

type pasteOutput struct {
	Text string `json:"text"           jsonschema:"current clipboard contents, verbatim"`
	Note string `json:"note,omitempty" jsonschema:"set only when text is empty: names why an empty result is expected"`
}

// pasteView shapes the tool output: empty text gets the note naming why, so
// an agent can tell "nothing there" from "non-text content" without a
// follow-up call. Pure, so the test covers it without a real pasteboard.
func pasteView(text string) pasteOutput {
	out := pasteOutput{Text: text}
	if text == "" {
		out.Note = "the clipboard is empty or holds non-text content (an image or file); pbpaste renders text only"
	}
	return out
}

func handlePaste(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ pasteInput,
) (*mcp.CallToolResult, pasteOutput, error) {
	text, err := clip.New().Paste()
	if err != nil {
		return nil, pasteOutput{}, err
	}
	return nil, pasteView(text), nil
}
