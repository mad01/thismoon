// Package mcpformat names the response encodings the csl MCP tools can
// return and encodes a tool's typed output into one of them.
//
// Every csl MCP tool accepts a response_format parameter. The format is
// resolved as: tool parameter, else the mcp.response_format key in
// config.yaml, else Default. JSON keeps the go-sdk's structured output;
// every other format is rendered to a single text block, because Claude
// Code forwards only structuredContent to the model when both are present.
package mcpformat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Format names. Text is tool-specific where a tool has a dedicated renderer
// (ripgrep-style for searches) and falls back to MarkdownKV elsewhere.
const (
	Text       = "text"
	JSON       = "json"
	JSONL      = "jsonl"
	TOON       = "toon"
	CSV        = "csv"
	MarkdownKV = "markdown-kv"
	XML        = "xml"

	// Default is the built-in format when neither the tool call nor the
	// config file picks one.
	Default = Text
)

// names lists every format in the order docs and errors present them.
var names = []string{Text, JSON, JSONL, TOON, CSV, MarkdownKV, XML}

// Names returns the valid format names in presentation order.
func Names() []string {
	out := make([]string, len(names))
	copy(out, names)
	return out
}

// Valid reports whether name is a known format.
func Valid(name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// Resolve picks the effective format: the tool parameter when set, else the
// configured default, else Default. An unknown value in either source is an
// error naming where it came from, so a typo in config.yaml is loud rather
// than silently falling back.
func Resolve(param, configured string) (string, error) {
	if param != "" {
		if !Valid(param) {
			return "", fmt.Errorf("unknown response_format %q; valid: %s", param, strings.Join(names, ", "))
		}
		return param, nil
	}
	if configured != "" {
		if !Valid(configured) {
			return "", fmt.Errorf("config mcp.response_format %q is not a known format; valid: %s", configured, strings.Join(names, ", "))
		}
		return configured, nil
	}
	return Default, nil
}

// Encode renders v, a tool's typed output, in the named format. Text is not
// handled here: callers with a dedicated text renderer use it, and callers
// without one pass MarkdownKV. JSON returns compact JSON. Every other format
// renders from the marshaled JSON, so json tags, field order, and omitempty
// apply exactly as they do for JSON; TOON alone sorts keys alphabetically.
func Encode(format string, v any) (string, error) {
	if format == Text {
		return "", errors.New("mcpformat: text is rendered by the caller, not Encode")
	}
	if !Valid(format) {
		return "", fmt.Errorf("unknown response_format %q; valid: %s", format, strings.Join(names, ", "))
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", format, err)
	}
	switch format {
	case JSON:
		return string(data), nil
	case TOON:
		return encodeTOON(data)
	}
	root, err := parseTree(data)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", format, err)
	}
	return encodeTree(format, root)
}

// encodeTree dispatches the formats rendered from the ordered tree.
func encodeTree(format string, root *node) (string, error) {
	switch format {
	case JSONL:
		return encodeJSONL(root), nil
	case CSV:
		return encodeCSV(root)
	case MarkdownKV:
		return encodeMarkdownKV(root)
	case XML:
		return encodeXML(root)
	}
	return "", fmt.Errorf("encode %s: no tree encoder", format)
}
