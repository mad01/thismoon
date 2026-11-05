package vault

import "errors"

// Sentinel errors callers branch on with errors.Is. The CLI maps them to exit
// codes and the MCP server reports their text.
var (
	// ErrNotFound means the note (or title) the caller named does not exist.
	ErrNotFound = errors.New("vault: note not found")
	// ErrExists means a write or promote would overwrite an existing note.
	ErrExists = errors.New("vault: note already exists")
	// ErrInvalidPath means a path argument is empty, absolute, or escapes the vault.
	ErrInvalidPath = errors.New("vault: invalid path")
	// ErrUnknownTier means a tier name is not inbox, notes, outputs, or MOCs.
	ErrUnknownTier = errors.New("vault: unknown tier")
	// ErrUnknownVault means a vault name is not in the config.
	ErrUnknownVault = errors.New("vault: unknown vault")
	// ErrAmbiguous means a bare path or title names notes in more than one
	// vault; the message lists the qualified candidates.
	ErrAmbiguous = errors.New("vault: ambiguous note")
	// ErrReadOnly means a write, edit or promote targets a shared vault, or
	// no writable vault is configured.
	ErrReadOnly = errors.New("vault: read-only")
	// ErrImmutableID means an edit would change or remove a note's id.
	ErrImmutableID = errors.New("vault: note id is immutable")
)
