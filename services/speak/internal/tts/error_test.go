package tts

import (
	"net/http"
	"strings"
	"testing"
)

func TestKindForStatus(t *testing.T) {
	cases := map[int]Kind{
		http.StatusUnauthorized:        KindAuth,
		http.StatusForbidden:           KindAuth,
		http.StatusNotFound:            KindModel,
		http.StatusTooManyRequests:     KindQuota,
		http.StatusBadRequest:          KindUpstream,
		http.StatusInternalServerError: KindUpstream,
		http.StatusServiceUnavailable:  KindUpstream,
	}
	for status, want := range cases {
		if got := KindForStatus(status); got != want {
			t.Errorf("KindForStatus(%d) = %q, want %q", status, got, want)
		}
	}
}

// TestDetailPullsTheBackendsMessage covers the error shapes speak meets: the
// OpenAI shape OpenRouter uses, FastAPI's detail (mlx-audio), a bare message,
// plain text, and an oversized HTML page that must not flood the UI.
func TestDetailPullsTheBackendsMessage(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"openai shape",
			`{"error":{"message":"No auth credentials found","code":401}}`,
			"No auth credentials found",
		},
		{"string error", `{"error":"model not found"}`, "model not found"},
		{"fastapi detail", `{"detail":"LocalEntryNotFoundError"}`, "LocalEntryNotFoundError"},
		{
			"fastapi validation",
			`{"detail":[{"msg":"field required"}]}`,
			`[{"msg":"field required"}]`,
		},
		{"bare message", `{"message":"quota exceeded"}`, "quota exceeded"},
		{"plain text", "  Internal\n  Server Error ", "Internal Server Error"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detail([]byte(tc.body)); got != tc.want {
				t.Errorf("Detail = %q, want %q", got, tc.want)
			}
		})
	}

	long := Detail([]byte("<html>" + strings.Repeat("é", 400) + "</html>"))
	if len(long) > maxDetailBytes+len("…") || !strings.HasSuffix(long, "…") {
		t.Errorf(
			"long body detail is %d bytes, want capped at %d with an ellipsis",
			len(long),
			maxDetailBytes,
		)
	}
}

func TestFromResponse(t *testing.T) {
	got := FromResponse(
		"local",
		"TTS engine",
		http.StatusNotFound,
		[]byte(`{"detail":"no voice xx"}`),
	)
	if got.Kind != KindModel || got.Status != http.StatusNotFound || got.Provider != "local" {
		t.Errorf("FromResponse = %+v, want a 404 model error from local", got)
	}
	if got.Message != "TTS engine returned 404: no voice xx" {
		t.Errorf("message = %q", got.Message)
	}
	if bare := FromResponse("local", "TTS engine", 500, nil); bare.Message != "TTS engine returned 500" {
		t.Errorf("message without body = %q", bare.Message)
	}
}
