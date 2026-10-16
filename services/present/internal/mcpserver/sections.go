package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/docedit"
)

// patchSectionDescription teaches the tool, with the one example a client
// needs to get the nested JSON string right.
const patchSectionDescription = "Change one section of a page without resending the whole Doc. " +
	"op replace, insert_before, insert_after, or remove acts at the section `anchor` names: " +
	"its table-of-contents anchor (the heading lowercased, spaces as dashes, other punctuation dropped, so \"Next steps\" is next-steps; a heading is accepted too) " +
	"or index:N, zero-based, for a heading that repeats or has no letters. " +
	"op append adds a section at the end and takes no anchor. " +
	"`section` is one Doc section as a JSON string ({h, blocks, tone?, layout?, notes?, reveal?}, the shape present_create's content sections take), " +
	"or markdown opening with a # or ## heading when body_format is markdown (one section; ### and deeper become subheadings; a mermaid fence stays code). " +
	"rendition picks the brief (content, the default) or the deck. " +
	"Pass base_version, the version you last read, and the patch is refused with the current version if the page changed since. " +
	"The whole Doc is recompiled, so a section the renderer refuses is refused here. " +
	"An anchor several sections share is refused with their indices; a heading that would repeat another section's anchor is refused. " +
	"Returns the new version and where the section landed. " +
	`Example: {"id":"3f2a9c1b7e","op":"replace","anchor":"next-steps","base_version":4,` +
	`"section":"{\"h\":\"Next steps\",\"blocks\":[{\"t\":\"list\",\"items\":[\"Ship the patch route\",\"Add history\"]}]}"}`

type patchSectionInput struct {
	ID          string `json:"id"                     jsonschema:"page id"`
	Op          string `json:"op"                     jsonschema:"replace | insert_before | insert_after | remove | append"`
	Anchor      string `json:"anchor,omitempty"       jsonschema:"the section to act at: its table-of-contents anchor or index:N (zero-based); omit for append"`
	Section     string `json:"section,omitempty"      jsonschema:"one Doc section as a JSON string, or markdown when body_format is markdown; omit for remove"`
	BodyFormat  string `json:"body_format,omitempty"  jsonschema:"json (default) or markdown"`
	Rendition   string `json:"rendition,omitempty"    jsonschema:"content (default) or deck"`
	BaseVersion int    `json:"base_version,omitempty" jsonschema:"the page version the patch was written against; the patch is refused if the page has moved on"`
}

type patchSectionOutput struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Rendition string `json:"rendition"`
	Op        string `json:"op"`
	Index     int    `json:"index"     jsonschema:"zero-based index of the section written, or of the one removed"`
	Anchor    string `json:"anchor"    jsonschema:"anchor of the section written, or of the one removed"`
	URL       string `json:"url"`
}

func registerSectionTool(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "present_patch_section",
		Description: patchSectionDescription,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(true),
			IdempotentHint:  false,
			OpenWorldHint:   new(false),
		},
	}, withHint(h.handlePatchSection))
}

// handlePatchSection parses the patch and runs it as one store edit. On a
// shared instance only the page's author may patch it. Image files the
// section names become stored images once the section compiles on its own;
// the edit compiles the whole Doc again.
func (h *handlers) handlePatchSection(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in patchSectionInput,
) (*mcp.CallToolResult, patchSectionOutput, error) {
	patch, err := docedit.Parse(docedit.Request{
		Op: in.Op, Anchor: in.Anchor, Rendition: in.Rendition, BaseVersion: in.BaseVersion,
		BodyFormat: in.BodyFormat, Section: []byte(in.Section),
	})
	if err != nil {
		return nil, patchSectionOutput{}, err
	}
	cur, err := h.store.Get(ctx, in.ID)
	if err != nil {
		return nil, patchSectionOutput{}, err
	}
	if h.mode == ModeShared {
		if err := author.Check(cur.Author, header(req)); err != nil {
			return nil, patchSectionOutput{}, err
		}
	}
	if patch, err = h.ingestSection(patch, cur.Title); err != nil {
		return nil, patchSectionOutput{}, err
	}
	var res docedit.Result
	p, err := h.store.Edit(ctx, in.ID, h.renewing(patch.Edit(&res)))
	if err != nil {
		return nil, patchSectionOutput{}, err
	}
	notify.EmitEvent("present", "info", "page section patched: "+p.Title, "",
		map[string]string{"id": p.ID, "title": p.Title, "anchor": res.Anchor})
	url := h.url(req, p.ID)
	if patch.Rendition == docedit.Deck {
		url = deckURL(url)
	}
	return nil, patchSectionOutput{
		ID: p.ID, Version: p.Version, Rendition: string(patch.Rendition), Op: string(patch.Op),
		Index: res.Index, Anchor: res.Anchor, URL: url,
	}, nil
}

// ingestSection turns the section's local image files into stored images,
// checks that the section compiles on its own, and only then writes the
// files, so a refused section leaves no file behind.
func (h *handlers) ingestSection(
	patch docedit.SectionPatch,
	title string,
) (docedit.SectionPatch, error) {
	if patch.Op == docedit.OpRemove {
		return patch, nil
	}
	ing := h.newImageIngest()
	section, err := ing.section(patch.Section)
	if err != nil {
		return docedit.SectionPatch{}, err
	}
	patch.Section = section
	if err := patch.Check(title); err != nil {
		return docedit.SectionPatch{}, err
	}
	if err := ing.commit(); err != nil {
		return docedit.SectionPatch{}, err
	}
	return patch, nil
}
