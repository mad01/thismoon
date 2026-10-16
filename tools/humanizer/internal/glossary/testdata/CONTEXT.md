# fixture

_Avoid_: orphan (an Avoid line above every term is ignored)

## Language

**Component**:
A releasable unit with its own release line.
_Avoid_: module, package (both mean something else in Go, see the notes)

**Service**:
A component that runs locally.
_Avoid_: app, daemon, component

**Deck:**
The slide rendition of a page.
_Avoid_: slideshow, `presentation`, version (the revision counter)

**Presenting**:
The deck view with the chrome hidden.
_Avoid_: fullscreen, Slideshow mode, slideshow

**Author key**:
A self-issued secret.
_Avoid_: token (how the key travels, not what it is), API key, it's-a-key

**Draft**:
A term with no Avoid line.
