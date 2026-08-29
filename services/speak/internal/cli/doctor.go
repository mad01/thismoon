package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/doctor"
	speak "github.com/mad01/thismoon/services/speak"
)

// engineProbeTimeout bounds the TTS engine ping; matches the budget serve's
// /enginez handler gives the same upstream.
const engineProbeTimeout = 1500 * time.Millisecond

func init() {
	rootCmd.AddCommand(agentcli.DoctorCommand(speak.Facts(), doctorChecks))
}

// doctorChecks builds the check list at run time, so it probes the resolved
// --port/--tts-url/--state-dir (env vars included) rather than the
// compile-time defaults. `speak doctor` and the speak_doctor MCP tool both
// run it, so a shell and an agent see the same diagnosis.
//
// The engine check leads: the MCP tools speak through the engine alone, so
// the web surface's checks failing still leaves the tools able to talk.
func doctorChecks(context.Context) []doctor.Check {
	baseURL := serveBaseURL()
	return []doctor.Check{
		engineReachable(flagTTSURL),
		stateDirReadable(flagStateDir),
		doctor.ServiceReachable(baseURL),
		doctor.VersionSkew(baseURL),
	}
}

// stateDirReadable is doctor.StoreReadable with one case carved out: the
// playback engine creates the state directory the first time it synthesizes
// audio, so on an install that has never played anything the directory is
// simply not there yet. That is not a fault to report — the shared check
// would call it "store not readable", which reads like a permissions or
// disk problem — so it skips with the path instead. Anything else that
// makes the directory unopenable still fails.
func stateDirReadable(path string) doctor.Check {
	check := doctor.StoreReadable(path)
	inner := check.Run
	check.Run = func(ctx context.Context) error {
		if path != "" {
			expanded, err := confdir.Expand(path)
			if err == nil {
				if _, statErr := os.Stat(expanded); errors.Is(statErr, fs.ErrNotExist) {
					return doctor.Skip("no playback state yet at " + expanded +
						"; created on the first speak_text or speak_file call")
				}
			}
		}
		return inner(ctx)
	}
	return check
}

// engineReachable probes the TTS engine the way serve's /enginez handler
// does: GET the engine root and count any HTTP response, even a 404, as
// reachable.
func engineReachable(ttsURL string) doctor.Check {
	return doctor.Check{
		Name: "tts-engine-reachable",
		Run: func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, engineProbeTimeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ttsURL+"/", nil)
			if err != nil {
				return fmt.Errorf("build engine probe: %w", err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("TTS engine not reachable at %s: %w", ttsURL, err)
			}
			return res.Body.Close()
		},
	}
}
