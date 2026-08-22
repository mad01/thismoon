package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	catalogroot "github.com/mad01/thismoon/services/catalog"
	"github.com/mad01/thismoon/services/catalog/internal/catalog"
)

var validateCmd = &cobra.Command{
	Use:   "validate [path...]",
	Short: "Validate service-info.yaml files and global name uniqueness",
	Long: `Validate catalog entities.

Each service-info.yaml is parsed and schema-checked (kind, name, owner, and a
system link for components). Then the global-namespace rule is enforced: every
name must be unique across all Systems and Components combined.

With no arguments, validation runs over the repos in the registry. With one or
more path arguments (directories or service-info.yaml files), only those are
validated — this is the mode the catalog repo's PR check uses.`,
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	var entities []catalog.Entity
	if len(args) == 0 {
		// Hint only environment errors (registry, scan roots); a schema or
		// uniqueness failure below is the command's finding, not a breakage.
		reg, err := catalog.LoadRegistry(registryPath)
		if err != nil {
			return agentdoc.Hint(err, catalogroot.Facts())
		}
		entities, err = catalog.ScanPaths(ctx, reg.Paths())
		if err != nil {
			return agentdoc.Hint(err, catalogroot.Facts())
		}
	} else {
		for _, p := range args {
			ents, err := validatePath(ctx, p)
			if err != nil {
				return err
			}
			entities = append(entities, ents...)
		}
	}

	if err := catalog.CheckUnique(entities); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "ok: %d entities valid, names globally unique\n", len(entities))
	return nil
}

// validatePath scans a directory or parses a single service-info.yaml file.
func validatePath(ctx context.Context, path string) ([]catalog.Entity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return catalog.ScanDir(ctx, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ents, err := catalog.ParseEntities(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i := range ents {
		ents[i].SourcePath = path
	}
	return ents, nil
}
