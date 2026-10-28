package hint

import "encoding/json"

// ReadTotalLines reads the whole file's line count from a Read tool
// response: file.totalLines on a response of type "text". Any other shape
// returns 0, which callers read as unknown: an image, notebook or PDF read,
// a missing field, a field of another JSON type, or no JSON object at all.
func ReadTotalLines(raw json.RawMessage) int {
	var resp struct {
		Type string `json:"type"`
		File *struct {
			TotalLines int `json:"totalLines"`
		} `json:"file"`
	}
	if json.Unmarshal(raw, &resp) != nil || resp.Type != "text" || resp.File == nil {
		return 0
	}
	return max(resp.File.TotalLines, 0)
}
