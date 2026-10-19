# present

Briefing pages, decks, and the shared instance. These terms apply to the files
under `services/present/`; the repo-wide terms are in the root CONTEXT.md.

## Language

**Shared instance**:
A present running with `--shared`: it serves pages by id only, takes writes only with an author key, and keeps no index or listing. Where a local present shares pages to.
_Avoid_: shared server, remote present

**Author key**:
A self-issued secret a client sends to a shared instance as a bearer token; the instance stores only its hash, which becomes the page's author. Minted with `present key new`.
_Avoid_: token (the bearer token is how the key travels, not what it is), password, API key

**Ephemeral page**:
A shared page that expires 30 days after its last write or share and is then purged by the sweeper.
_Avoid_: temporary page, TTL page

**Share**:
Pushing a copy of a local page to a shared instance. Sharing the same page again replaces the copy under the same link.
_Avoid_: publish, upload, sync

**Brief**:
The scrollable rendition of a present page: the Doc compiled to `content.html`, read top to bottom. The rendition present always had; a page may carry it, a deck, or both.
_Avoid_: page content (ambiguous once a deck exists), article, document

**Deck**:
The slide rendition of a present page. A second Doc, authored on its own and compiled to `deck.html` beside the brief under the same page id, shown one section at a time. Never derived from the brief; either rendition can exist without the other.
_Avoid_: slideshow, presentation (present's pages are all presentations), version (the revision counter)

**Slide**:
One section of a deck as the deck view shows it. The deck Doc's title, summary, meta, and chips make the title slide; its references make the last one. A slide paints no filled box of its own: the slide is already the container, and a tone colours its accent.
_Avoid_: page (a page is the whole id), frame

**Presenting**:
The deck view's state with the chrome hidden and one slide filling the window. It is entered and left with the Present button, the F or P key, Escape, or a remote command from the present_deck tool or `present deck`.
_Avoid_: fullscreen (the browser's own fullscreen is one optional layer on top), slideshow mode

**Step**:
One press of Next on a slide before the deck moves on: a reveal item or block appearing, a stepped chart showing its next series with its caption, or a stepped diagram showing its next elements or focus. A ribbon chart steps by period instead. The counter shows a slide's steps as `s/S`. A jump lands with every step taken.
_Avoid_: fragment (the CSS class behind a reveal step), stage, build

**Ribbon chart**:
A chart kind for how categories rank over periods. One column per period stands on the baseline with the categories stacked inside it, by rank unless ordered as given. A ribbon joins each category's segments across adjacent columns, so a crossing is a rank change. It sits on the page with no card by default.
_Avoid_: alluvial, bump chart, stream graph, sankey (a sankey shows volume flowing between stages, not ranks over periods)

**Diagram**:
An architecture picture on a page: boxes with a name and a short text, labelled boundaries (groups) around them, and labelled arrows between the boxes. The browser lays it out and draws it as SVG with no card of its own. Many per page. A stepped diagram shows its elements step by step and lights up a step's focus.
_Avoid_: graph (the page's one network map), flowchart, architecture map

**Terminal block**:
A block with `t: "terminal"` that shows a terminal session inside a brief section or on a slide. It names the session, and for a session it starts, the command, the directory, and one line typed on the prompt. Every terminal block on a page that names one session shows the same process. On a slide it is the one card a deck keeps, since the live process is the slide's content.
_Avoid_: shell (in present the shell is the static HTML chrome: shell.html, pageShell, deckShell), console, terminal widget

**Terminal session**:
The process a local present serve runs in a pseudo-terminal (PTY) for one page under one session name. It starts the first time a terminal block naming it attaches, and it outlives the tab that showed it. A tab that attaches later replays its recent output and takes input again. Killing it ends it, and so does a long spell with no tab watching. Switching the live terminal off or stopping serve ends every session.
_Avoid_: shell session, console, terminal tab

**Live terminal**:
The feature that lets terminal blocks run terminal sessions. Off by default and local only: `present terminal on` creates the switch file `terminal.enabled` in the workdir, and `present terminal off` removes it and ends every session. A shared instance has no live terminal and shows a placeholder in each terminal block.
_Avoid_: remote shell, web console
