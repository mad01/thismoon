package cli

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	speak "github.com/mad01/thismoon/services/speak"
)

// engineProbeTimeout bounds the TTS engine ping; matches the budget serve's
// /enginez handler gives the same upstream.
const engineProbeTimeout = 1500 * time.Millisecond

func init() {
	// Checks build at run time so they probe the resolved --port/--tts-url
	// (env vars included), not the compile-time defaults. The engine check
	// leads: the MCP tools speak through the engine alone, so the web
	// surface's checks failing still leaves the tools able to talk.
	doctorCmd := agentcli.DoctorCommand(speak.Facts(), func(context.Context) []doctor.Check {
		baseURL := fmt.Sprintf("http://localhost:%d", flagPort)
		return []doctor.Check{
			engineReachable(flagTTSURL),
			doctor.StoreReadable(speak.DefaultStateDir),
			doctor.ServiceReachable(baseURL),
			doctor.VersionSkew(baseURL),
		}
	})
	doctorCmd.Flags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the speak serve web surface listens on (env SPEAK_PORT)")
	doctorCmd.Flags().StringVar(&flagTTSURL, "tts-url", resolvedTTSURL(),
		"base URL of the mlx-audio TTS server (env SPEAK_TTS_URL)")
	rootCmd.AddCommand(doctorCmd)
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
