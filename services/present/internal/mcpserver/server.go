// Package mcpserver wires the present store to MCP tools that create, read,
// update, and list presentations (no delete), plus open one in the browser.
// The same tools serve a shared instance over HTTP with a smaller set: no
// listing, no browser, and an author key on every write.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/doctor"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/sharedclient"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// Name is the MCP server name advertised to clients.
const Name = "present"

// Mode selects the tool set. It mirrors the HTTP server's mode without
// importing it, so the two packages stay independent.
type Mode int

const (
	// ModeLocal is present mcp over stdio beside a local serve: the full
	// tool set, URLs on the local base URL.
	ModeLocal Mode = iota
	// ModeShared runs inside a shared instance over HTTP: no present_list
	// or present_open, an author key on create and update, URLs derived
	// from each request's forwarded headers.
	ModeShared
)

// Config holds the runtime settings shared with the HTTP server so the URLs
// returned by tools match where pages are served.
type Config struct {
	Workdir string
	Port    int
	BaseURL string // override for URL prefix (e.g. "http://present.this"); falls back to http://localhost:<Port>

	// Store is the page store the tools read and write. When nil, the
	// filesystem store under Workdir is opened, which is what present mcp
	// does locally.
	Store store.Store

	// Mode selects the tool set; the zero value is ModeLocal.
	Mode Mode

	// Now is the clock ephemeral expiries are computed from; nil means
	// time.Now.
	Now func() time.Time

	// Sharer, in local mode, is the shared instance present_share pushes
	// pages to; nil leaves the tool unregistered.
	Sharer *sharedclient.Client

	// Checks builds the diagnostics behind the present_doctor tool, against
	// the same resolved flags the rest of the CLI uses. It is required: the
	// instructions block advertises present_doctor to every client, so a
	// server that could not register it must not start.
	Checks func(ctx context.Context) []doctor.Check
}

// New builds the present MCP server.
func New(version string, cfg Config) (*mcp.Server, error) {
	if cfg.Checks == nil {
		return nil, errors.New(
			"mcpserver: no doctor checks; present_doctor is advertised to clients and must be registered",
		)
	}
	st := cfg.Store
	if st == nil {
		fs, err := store.NewFS(cfg.Workdir)
		if err != nil {
			return nil, hint(err)
		}
		st = fs
	}
	// Locally the base URL is fixed at startup. A shared instance leaves it
	// empty unless overridden, and derives it per request instead.
	base := cfg.BaseURL
	facts := present.Facts()
	if cfg.Mode == ModeShared {
		facts = present.SharedFacts()
	} else if base == "" {
		base = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	s := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(facts)},
	)
	registerTools(s, &handlers{
		store:   st,
		mode:    cfg.Mode,
		baseURL: base,
		now:     now,
		open:    openURL,
		checks:  cfg.Checks,
		sharer:  cfg.Sharer,
	})
	return s, nil
}
