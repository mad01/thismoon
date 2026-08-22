package cli

import (
	"context"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	catalogroot "github.com/mad01/thismoon/services/catalog"
	"github.com/mad01/thismoon/services/catalog/internal/catalog"
)

func init() {
	// Checks build at run time so they probe the resolved --registry, not the
	// compile-time default. The two web probes hit the default port: the web
	// process is optional, and the CLI works without it.
	rootCmd.AddCommand(agentcli.DoctorCommand(catalogroot.Facts(), func(context.Context) []doctor.Check {
		baseURL := catalogroot.Facts().BaseURL
		return []doctor.Check{
			doctor.StoreReadable(registryPath),
			registryLoads(),
			doctor.ServiceReachable(baseURL),
			doctor.VersionSkew(baseURL),
		}
	}))
}

// registryLoads runs the same load `catalog list` uses, registry parse plus a
// full scan, so the one-bad-service-info.yaml-aborts-everything case fails
// here with the file named.
func registryLoads() doctor.Check {
	return doctor.Check{
		Name: "registry-loads",
		Run: func(ctx context.Context) error {
			_, _, err := catalog.Load(ctx, registryPath)
			return err
		},
	}
}
