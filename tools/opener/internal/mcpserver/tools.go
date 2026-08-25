package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/opener/internal/sysopen"
)

func registerTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "open_url",
		Description: "Open a URL in the user's default browser (macOS open). " +
			"Use when the user says open this link, open in browser, or take me to that page. " +
			"The URL must carry a scheme (https://...); non-http schemes go to their default " +
			"handler. Returns the URL it opened.",
	}, handleURL)

	mcp.AddTool(s, &mcp.Tool{
		Name: "open_file",
		Description: "Open a file or directory with its default macOS application (open <path>). " +
			"Use when the user says open this file, open that folder, or open it in the default " +
			"app. The path must be absolute or ~-prefixed and must exist. Returns the path it opened.",
	}, handleFile)

	mcp.AddTool(s, &mcp.Tool{
		Name: "open_app",
		Description: "Launch or foreground a macOS application by name (open -a), e.g. Safari, " +
			"Xcode, Finder. Use when the user says launch, start, or switch to an app.",
	}, handleApp)

	mcp.AddTool(s, &mcp.Tool{
		Name: "open_with",
		Description: "Open a file with a specific macOS application instead of its default " +
			"(open -a <app> <path>). Use when the user says open this in some app, e.g. open the " +
			"log in Visual Studio Code. The path must be absolute or ~-prefixed and must exist.",
	}, handleWith)

	mcp.AddTool(s, &mcp.Tool{
		Name: "reveal_in_finder",
		Description: "Reveal a file or directory in a Finder window, selected (open -R). " +
			"Use when the user says show in Finder, reveal this file, or where is this on disk. " +
			"The path must be absolute or ~-prefixed and must exist.",
	}, handleReveal)
}

// openedOutput confirms what was handed to the open command, resolved: the
// URL verbatim, paths after ~ expansion, app names as given.
type openedOutput struct {
	Opened string `json:"opened" jsonschema:"the URL, resolved path, or app name that was opened"`
}

type urlInput struct {
	URL string `json:"url" jsonschema:"the URL to open; must carry a scheme, e.g. https://example.com"`
}

func handleURL(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in urlInput,
) (*mcp.CallToolResult, openedOutput, error) {
	opened, err := sysopen.New().URL(in.URL)
	if err != nil {
		return nil, openedOutput{}, err
	}
	return nil, openedOutput{Opened: opened}, nil
}

type fileInput struct {
	Path string `json:"path" jsonschema:"absolute or ~-prefixed path of the file or directory to open"`
}

func handleFile(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in fileInput,
) (*mcp.CallToolResult, openedOutput, error) {
	opened, err := sysopen.New().File(in.Path)
	if err != nil {
		return nil, openedOutput{}, err
	}
	return nil, openedOutput{Opened: opened}, nil
}

type appInput struct {
	Name string `json:"name" jsonschema:"application name as Launch Services knows it, e.g. Safari, Xcode"`
}

func handleApp(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in appInput,
) (*mcp.CallToolResult, openedOutput, error) {
	opened, err := sysopen.New().App(in.Name)
	if err != nil {
		return nil, openedOutput{}, err
	}
	return nil, openedOutput{Opened: opened}, nil
}

type withInput struct {
	Path string `json:"path" jsonschema:"absolute or ~-prefixed path of the file to open"`
	App  string `json:"app"  jsonschema:"application to open it with, e.g. Visual Studio Code"`
}

func handleWith(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in withInput,
) (*mcp.CallToolResult, openedOutput, error) {
	opened, err := sysopen.New().With(in.Path, in.App)
	if err != nil {
		return nil, openedOutput{}, err
	}
	return nil, openedOutput{Opened: opened}, nil
}

type revealInput struct {
	Path string `json:"path" jsonschema:"absolute or ~-prefixed path to reveal in Finder"`
}

func handleReveal(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in revealInput,
) (*mcp.CallToolResult, openedOutput, error) {
	opened, err := sysopen.New().Reveal(in.Path)
	if err != nil {
		return nil, openedOutput{}, err
	}
	return nil, openedOutput{Opened: opened}, nil
}
