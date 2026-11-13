# Blocks: layout blocks, images, links, and many items

SKILL.md's **Block types** lists every block and its fields. This file shows the layout blocks as they go in a section, how to lay out a `kv`, how an image is stored, how prose links to code and sources, and how a page holds many related items.

## Layout blocks

One example of each, as they go in a section's `blocks`:

```json
[
  {"t": "columns", "cols": [
    [{"t": "stat", "value": "41 min", "label": "checkout outage", "sub": "Tuesday 14:02 to 14:43"}],
    [{"t": "stat", "value": "3 min", "label": "to detect after the fix"}],
    [{"t": "stat", "value": "2", "label": "actions"}]
  ]},
  {"t": "columns", "cols": [
    [{"t": "chart", "kind": "area", "title": "Checkout 5xx per minute", "series": [{"points": [{"x": "14:00", "y": 0}, {"x": "14:15", "y": 940}]}]}],
    [{"t": "p", "text": "The spike starts eleven minutes before the first page."}]
  ]},
  {"t": "columns", "cols": [
    [{"t": "graph"}],
    [{"t": "p", "text": "The resolver sits between the gateway and payments; every request after 14:02 followed its stale record."}]
  ]},
  {"t": "columns", "cols": [
    [{"t": "image", "src": "https://example.com/dashboard.png", "alt": "The on-call dashboard at 14:15, every checkout panel red", "caption": "The dashboard at 14:15."}],
    [{"t": "p", "text": "Every checkout panel went red at once while the gateway's own panels stayed green."}]
  ]},
  {"t": "stat", "value": "41 min", "label": "checkout outage", "sub": "Tuesday 14:02 to 14:43"},
  {"t": "quote", "text": "We never saw the resolver because nothing watched it.", "cite": "On-call engineer, retrospective"},
  {"t": "details", "summary": "Full timeline, 14:00 to 15:00", "blocks": [
    {"t": "list", "items": ["14:02 first 502s at the edge", "14:09 paged", "14:43 resolved"]}
  ]},
  {"t": "callout", "sev": "ok", "text": "The TTL cap is live in every region."}
]
```

## Key-value blocks

Keep a `kv` key to about 15 characters: a longer key runs into its value. A `p` right after a `kv` sits flush against it, so put the paragraph that states the point before the `kv`.

```json
[
  {"t": "p", "text": "Three settings decide how long a stale record lives."},
  {"t": "kv", "kv": [{"k": "Time to live", "v": "300 s"}, {"k": "Retry count", "v": "3"}, {"k": "Timeout", "v": "2 s"}]}
]
```

## Images

`src` is an `http(s)` URL, or on this machine an absolute or `~` path to a png, jpeg, gif, or webp file of at most 2 MiB. The tool copies the file into the page store and rewrites `src` to the `/img/<hash>.<ext>` path it is served at, which is what `present_source` returns. A relative path is refused. A shared instance takes image URLs only, and a page with stored images can't be shared until they are URLs.

Write the `alt` as the sentence a listener needs in place of the picture: read-aloud reads it, then the caption. An image keeps a border unless `frame: false`, for a screenshot that carries its own edge.

## Linking to code and sources

Prose that names a file, function, PR, ticket, or commit links to it. The label is the short form the reader scans; the URL carries the repo and the path. Never write a bare `org/repo/path/to/file.go` in a text field.

```json
{"t": "p", "text": "The rank gap is widened in [app.js:26](https://github.com/mad01/thismoon/blob/1e2f309/services/present/internal/server/app.js#L26-L48), see [PR #164](https://github.com/mad01/thismoon/pull/164)."}
```

Rules:

1. **Label with the short form.** `graph.go:47`, `RenderGraph`, `PR #164`, `MAD-357`. The reader sees where, the link says exactly where.
2. **Permalink at a commit, not a branch.** Build GitHub links from `git remote get-url origin` and `git rev-parse HEAD`: `https://github.com/<org>/<repo>/blob/<sha>/<path>#L<start>-L<end>`. A branch link points at the wrong lines as soon as the file changes.
3. **Local pages can link into csl.** `csl_show_file(repo, file, start_line, end_line)` returns a `csl.this` link that opens those lines in the local code viewer. Use it for a page that stays on this machine; use GitHub links for a page that will be shared.

`http`, `https`, `mailto`, relative, and fragment URLs render as links. Anything else stays literal text.

## Presenting multiple items

When presenting N related items (review findings, bugs, options, comparison points), put them in one page as N sections, never one page per item. The reader then has one contents list to scan and one page to come back to.

```json
{
  "summary": "Three issues found during the assessment, one of them a data race.",
  "meta": "2026-06-05 · Code review · 3 findings",
  "chips": [{"text": "3 items", "style": "stat"}],
  "sections": [
    {
      "h": "The poll loop races on its shared cursor",
      "blocks": [{"t": "p", "text": "Description of the issue..."}]
    },
    {
      "h": "A failed write never reaches the caller",
      "blocks": [{"t": "p", "text": "Description of the issue..."}]
    },
    {
      "h": "The cache stays stale after a leader change",
      "blocks": [{"t": "p", "text": "Description of the issue..."}]
    }
  ]
}
```

Use this pattern for review findings, bug lists, options comparisons, and enumerable sets of any kind.

## What the lint catches in inline text

The pair below is generated from the lint's own test fixtures: a fragment that trips the rule, the warning the lint returns for it, and the same fragment repaired. SKILL.md's **Inline markdown** says which fields take markdown and which print the marks as typed.

<!-- pair:inline.stray_mark -->
This trips `inline.stray_mark`:

```json
{
  "title": "Releasing: from merged PR to signed artifact",
  "content": {
    "summary": "A merged PR reaches a signed artifact with no hand-made tag: release-please opens the release PR, and merging it cuts the tag.",
    "sections": [
      {
        "h": "`commit-search-depth` limits how far release-please looks back",
        "blocks": [
          {
            "t": "p",
            "text": "release-please walks main back from the head and stops after `commit-search-depth` commits, 200 here. A component whose last tag is further back is not seen."
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `content.sections[0].h` section "`commit-search-depth` limits how far release-please looks back", heading: a backtick in text shown as written. The marks print as they are, so the reader sees raw markdown instead of code or bold. Drop the marks: this field does not read markdown.

Repaired:

```json
{
  "title": "Releasing: from merged PR to signed artifact",
  "content": {
    "summary": "A merged PR reaches a signed artifact with no hand-made tag: release-please opens the release PR, and merging it cuts the tag.",
    "sections": [
      {
        "h": "release-please looks back only 200 commits",
        "blocks": [
          {
            "t": "p",
            "text": "release-please walks main back from the head and stops after `commit-search-depth` commits, 200 here. A component whose last tag is further back is not seen."
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->
