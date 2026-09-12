package mcpformat

import (
	"encoding/json"
	"fmt"

	"github.com/alpkeskin/gotoon"
)

// encodeTOON renders the marshaled JSON with gotoon. The bytes are decoded
// into plain maps and slices first instead of handing gotoon the struct:
// gotoon keys structs on the raw json tag, ",omitempty" included, and never
// drops empty fields. It sorts map keys alphabetically, so TOON is the one
// format that does not follow struct field order. gotoon emits no trailing
// newline; one is added so every format ends the same way.
func encodeTOON(data []byte) (string, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("mcpformat: decode for toon: %w", err)
	}
	out, err := gotoon.Encode(v)
	if err != nil {
		return "", fmt.Errorf("mcpformat: encode toon: %w", err)
	}
	return out + "\n", nil
}
