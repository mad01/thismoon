---
id: ambiguous-present-readme-intro
label: unknown
bucket: ambiguous
source: thismoon services/present/README.md lines 1-5, commit 7c6002d1f60e (2026-10-07)
license: BSD-3-Clause (this repository)
generator: ""
words: 152
notes: AI-drafted then humanized component README intro (MAD-337/MAD-340), the production traffic the judge sees.
---
# present

A small CLI + MCP server for serving single-page HTML briefing pages over localhost, and the same binary run as a shared instance others reach by link. A page can also carry a slide deck beside its brief, or instead of it, for the same material talked through in a room.

Pages are created, read, updated, and listed through the MCP tools; deleting one is a button in the web index rather than a tool. Each page keeps its rendered HTML body next to the Doc JSON it was rendered from. The browser does the assembly: `present serve` returns a chrome-only shell, fetches the page as JSON, and builds the DOM, so there is no template on disk to edit. An open tab reloads itself after an update: a shared instance on the cluster store pushes the new version over server-sent events, and everywhere else the tab polls for it.
