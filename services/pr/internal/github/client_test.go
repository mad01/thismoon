package github

import "testing"

func TestGhAPIErrorReason(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "self approval string errors",
			body: `{"message":"Unprocessable Entity","errors":["Review Can not approve your own pull request"],"status":"422"}`,
			want: "Review Can not approve your own pull request",
		},
		{
			name: "object errors with message",
			body: `{"message":"Validation Failed","errors":[{"resource":"PullRequestReview","message":"body is required"}]}`,
			want: "body is required",
		},
		{
			name: "object errors with field and code",
			body: `{"message":"Validation Failed","errors":[{"resource":"Issue","field":"title","code":"missing_field"}]}`,
			want: "title: missing_field",
		},
		{
			name: "multiple errors joined",
			body: `{"message":"Bad","errors":["first","second"]}`,
			want: "first; second",
		},
		{
			name: "falls back to top-level message",
			body: `{"message":"Not Found"}`,
			want: "Not Found",
		},
		{
			name: "non-JSON body",
			body: `gh: something broke`,
			want: "",
		},
		{
			name: "empty body",
			body: ``,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ghAPIErrorReason([]byte(tt.body)); got != tt.want {
				t.Errorf("ghAPIErrorReason() = %q, want %q", got, tt.want)
			}
		})
	}
}
