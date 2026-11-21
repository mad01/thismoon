package cli

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/tools/cards"
)

// fakeTTY is an opened terminal that records whether it was closed.
type fakeTTY struct {
	io.Writer
	closed bool
}

func (f *fakeTTY) Close() error {
	f.closed = true
	return nil
}

func TestDoctorChecks(t *testing.T) {
	noTTY := errors.New("open /dev/tty: device not configured")
	tests := []struct {
		name    string
		openErr error
		environ []string
		profile colorprofile.Profile
		// want is each check's status, in order: terminal, TERM, colors.
		want []string
		// wantDetail is a substring of the first failing check's detail.
		wantDetail string
	}{
		{
			name:    "a usable terminal passes",
			environ: []string{"TERM=xterm-256color"},
			profile: colorprofile.ANSI256,
			want:    []string{doctor.StatusOK, doctor.StatusOK, doctor.StatusOK},
		},
		{
			name:    "16 colors is enough",
			environ: []string{"TERM=xterm"},
			profile: colorprofile.ANSI,
			want:    []string{doctor.StatusOK, doctor.StatusOK, doctor.StatusOK},
		},
		{
			name:       "no terminal fails and skips the color check",
			openErr:    noTTY,
			environ:    []string{"TERM=xterm-256color"},
			profile:    colorprofile.TrueColor,
			want:       []string{doctor.StatusFail, doctor.StatusOK, doctor.StatusSkipped},
			wantDetail: "device not configured",
		},
		{
			name:       "TERM unset fails",
			environ:    []string{"HOME=/home/someone"},
			profile:    colorprofile.ANSI256,
			want:       []string{doctor.StatusOK, doctor.StatusFail, doctor.StatusOK},
			wantDetail: "TERM is not set",
		},
		{
			name:       "TERM empty fails",
			environ:    []string{"TERM="},
			profile:    colorprofile.ANSI256,
			want:       []string{doctor.StatusOK, doctor.StatusFail, doctor.StatusOK},
			wantDetail: "TERM is not set",
		},
		{
			name:       "dumb TERM fails",
			environ:    []string{"TERM=dumb"},
			profile:    colorprofile.NoTTY,
			want:       []string{doctor.StatusOK, doctor.StatusFail, doctor.StatusFail},
			wantDetail: "TERM is dumb",
		},
		{
			name:       "the last TERM entry wins",
			environ:    []string{"TERM=xterm-256color", "TERM=dumb"},
			profile:    colorprofile.ANSI256,
			want:       []string{doctor.StatusOK, doctor.StatusFail, doctor.StatusOK},
			wantDetail: "TERM is dumb",
		},
		{
			name:       "no colors fails",
			environ:    []string{"TERM=xterm-256color", "NO_COLOR=1"},
			profile:    colorprofile.ASCII,
			want:       []string{doctor.StatusOK, doctor.StatusOK, doctor.StatusFail},
			wantDetail: "detected Ascii",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opened []*fakeTTY
			term := terminal{
				open: func() (io.WriteCloser, error) {
					if tc.openErr != nil {
						return nil, tc.openErr
					}
					f := &fakeTTY{Writer: io.Discard}
					opened = append(opened, f)
					return f, nil
				},
				environ: tc.environ,
				profile: func(io.Writer, []string) colorprofile.Profile { return tc.profile },
			}

			report := doctor.Collect(context.Background(), term.checks())

			got := make([]string, len(report.Checks))
			detail := ""
			for i, c := range report.Checks {
				got[i] = c.Status
				if c.Status == doctor.StatusFail && detail == "" {
					detail = c.Detail
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("statuses = %v, want %v (report %+v)", got, tc.want, report.Checks)
			}
			if !strings.Contains(detail, tc.wantDetail) {
				t.Errorf("first failure detail = %q, want it to contain %q", detail, tc.wantDetail)
			}
			for i, f := range opened {
				if !f.closed {
					t.Errorf("terminal opened by check %d was not closed", i+1)
				}
			}
		})
	}
}

func TestDoctorNamesTERM(t *testing.T) {
	term := terminal{environ: []string{"TERM=xterm-kitty"}}
	if got := term.checks()[1].Name; got != "TERM (xterm-kitty)" {
		t.Errorf("TERM check name = %q, want %q", got, "TERM (xterm-kitty)")
	}
}

func TestDoctorFailureReturnsError(t *testing.T) {
	term := terminal{
		open:    func() (io.WriteCloser, error) { return nil, errors.New("no terminal") },
		environ: nil,
		profile: func(io.Writer, []string) colorprofile.Profile { return colorprofile.NoTTY },
	}
	var out strings.Builder
	err := doctor.Run(context.Background(), &out, cards.Facts(), term.checks())
	if err == nil {
		t.Fatalf("doctor.Run with no terminal returned nil, want an error\n%s", out.String())
	}
	if !strings.Contains(out.String(), "FAIL terminal (/dev/tty): no terminal") {
		t.Errorf("doctor output does not report the terminal failure:\n%s", out.String())
	}
}
