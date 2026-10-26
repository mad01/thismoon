package hook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

// PayloadDenyID labels the denial a payload belt cannot read produces, in
// the same belt[<id>] form the guards use.
const PayloadDenyID = "payload"

// decodeErrorTag carries why a payload did not decode, on the warn event
// the guard and hint entrypoints archive for it.
const decodeErrorTag = "decode_error"

// guardInputFields lists, per guard event, the tool_input keys toInput
// reads as strings. Keep it in step with toInput: a key missing here is
// read as "" when it arrives with another type, and the guards then check
// a command or a path that is not the one about to run.
var guardInputFields = map[string][]string{
	guard.EventBash:  {"command"},
	guard.EventWrite: {"file_path", "content", "new_string"},
}

// hintInputFields is the same list for toHintInput. The read event's offset
// and limit are left out on purpose: they are JSON numbers, and toHintInput
// reads only whether they are present.
var hintInputFields = map[string][]string{
	hint.EventBash:   {"command"},
	hint.EventSearch: {"repo"},
	hint.EventRead:   {"file_path"},
}

// fieldTypeError is a payload field that arrived with the wrong JSON type,
// named by its key path in the payload rather than by Go's struct names.
type fieldTypeError struct {
	field string // key path, e.g. "cwd" or "tool_input.command"
	got   string // the JSON type that arrived
	want  string // the JSON type belt reads, with its article
}

func (e *fieldTypeError) Error() string {
	return fmt.Sprintf("field %s is a JSON %s, want %s", e.field, e.got, e.want)
}

// decodePayload reads one hook payload from r. It fails when r holds no
// JSON object, when a top-level field the entrypoints read (tool_name,
// tool_input, cwd) has the wrong JSON type, or when a tool_input key named
// in fields is not a string. A JSON null counts as absent, the way
// encoding/json treats it. The identity and session fields decode
// leniently and never fail it (see lenientString). On a type error the
// returned payload still holds every field that did decode, so the warning
// that follows can name who made the call.
func decodePayload(r io.Reader, fields []string) (payload, error) {
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return payload{}, errors.New("empty payload")
		}
		return payload{}, fmt.Errorf("not JSON: %w", err)
	}
	if raw[0] != '{' {
		return payload{}, errors.New("not a JSON object")
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return p, &fieldTypeError{field: te.Field, got: te.Value, want: wantedType(te.Type)}
		}
		return p, fmt.Errorf("does not decode: %w", err)
	}
	for _, key := range fields {
		v := p.ToolInput[key]
		if _, ok := v.(string); v != nil && !ok {
			return p, &fieldTypeError{
				field: "tool_input." + key,
				got:   jsonType(v),
				want:  "a string",
			}
		}
	}
	return p, nil
}

// wantedType names the JSON type a strict payload field decodes into.
func wantedType(t reflect.Type) string {
	if t.Kind() == reflect.Map {
		return "an object"
	}
	return "a " + t.Kind().String()
}

// jsonType names the JSON type of a value decoded into any, in the words
// encoding/json uses for its own type errors.
func jsonType(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

// payloadDenial explains a tool call blocked because its payload could not
// be read: what failed, why that blocks, and why retrying does not help.
func payloadDenial(err error) *guard.Denial {
	return guard.Reasonf(
		PayloadDenyID,
		"belt could not read the hook payload for this tool call (%v), so no guard can check it "+
			"and the call is blocked. Retrying will not help: the harness builds the payload, "+
			"not your command. Tell the user; `belt docs` explains this failure.",
		err,
	)
}
