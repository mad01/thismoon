package hint

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// kofProbeTimeout bounds the doctor probe. Doctor is human-invoked, so it can
// afford more patience than the per-tool-call hint budget.
const kofProbeTimeout = 2 * time.Second

// KofBaseURL reports where the kof hints look for kof serve, so doctor can
// probe the same instance the hints query.
func KofBaseURL() string { return kofBaseURL() }

// KofProbe asks kof serve how many assertions it stores. Unlike the hints,
// which render "kof down" and "store empty" identically (as silence), it
// returns the error — telling those apart is what the doctor line exists for.
func KofProbe(base string) (int, error) {
	client := &http.Client{Timeout: kofProbeTimeout}
	resp, err := client.Get(base + "/api/assertions")
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("hint: kof probe status %s", resp.Status)
	}
	var body struct {
		Assertions []assertion `json:"assertions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("hint: decode kof probe response: %w", err)
	}
	return len(body.Assertions), nil
}
