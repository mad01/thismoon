package sample

// widgetInput is the MCP tool input, with a jsonschema description per
// field.
type widgetInput struct {
	Name  string `json:"name"            jsonschema:"the widget name; also the file it is stored under"`
	Notes string `json:"notes,omitempty" jsonschema:"free text kept beside the widget"`
	count int
}

type tool struct {
	Name        string
	Description string
}

var widgetTool = tool{
	Name: "sample_widget",
	Description: "Build a widget from a name and return where it landed. " +
		"Pass the name the caller typed, not a slug.",
}
