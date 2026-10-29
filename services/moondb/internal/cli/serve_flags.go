package cli

import (
	"errors"
	"fmt"
	"net"

	"github.com/spf13/pflag"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/services/moondb"
)

// The flags of serve beyond the persistent ones: the shared instance and
// replication. Every one is off by default, so a plain `moondb serve` is
// the loopback serve of one machine.
var (
	flagShared           bool
	flagBind             string
	flagBackend          string
	flagDataDir          string
	flagGCSBucket        string
	flagKeysFile         string
	flagReplicateTo      string
	flagReplicateKeyFile string
)

func addServeFlags(f *pflag.FlagSet) {
	f.BoolVar(&flagShared, "shared", envdefault.Bool("MOONDB_SHARED", false),
		"run as a shared instance: records from replicating serves, keys on every read "+
			"and write, MCP at /mcp (env MOONDB_SHARED)")
	f.StringVar(&flagBind, "bind", envdefault.String("MOONDB_BIND", ""),
		"host:port a shared instance listens on (env MOONDB_BIND); required with --shared")
	f.StringVar(&flagBackend, "backend", envdefault.String("MOONDB_BACKEND", moondb.DefaultBackend),
		"store backend of a shared instance: local or cloud (env MOONDB_BACKEND)")
	f.StringVar(&flagDataDir, "data-dir", envdefault.String("MOONDB_DATA_DIR", ""),
		"directory a shared instance keeps its log in (env MOONDB_DATA_DIR); default the workdir")
	f.StringVar(&flagGCSBucket, "gcs-bucket", envdefault.String("MOONDB_GCS_BUCKET", ""),
		"Cloud Storage bucket of the cloud backend: chunks and a copy of the log "+
			"(env MOONDB_GCS_BUCKET)")
	f.StringVar(&flagKeysFile, "keys-file", envdefault.String("MOONDB_KEYS_FILE", ""),
		"file of the hex SHA-256 hashes of the keys a shared instance accepts, one per line "+
			"(env MOONDB_KEYS_FILE)")
	f.StringVar(&flagReplicateTo, "replicate-to", envdefault.String("MOONDB_REPLICATE_TO", ""),
		"base URL of a shared instance this serve copies its store to (env MOONDB_REPLICATE_TO); "+
			"empty turns replication off")
	f.StringVar(&flagReplicateKeyFile, "replicate-key-file",
		envdefault.String("MOONDB_REPLICATE_KEY_FILE", ""),
		"file holding the key the shared instance accepts (env MOONDB_REPLICATE_KEY_FILE)")
}

// checkServeFlags refuses a combination of serve flags that cannot run,
// before anything opens, and expands a leading ~ in the paths.
func checkServeFlags() error {
	for _, p := range []*string{&flagDataDir, &flagKeysFile, &flagReplicateKeyFile} {
		expanded, err := confdir.Expand(*p)
		if err != nil {
			return err
		}
		*p = expanded
	}
	if flagShared {
		return checkSharedFlags()
	}
	return checkLocalFlags()
}

// checkLocalFlags keeps the shared instance's flags off a local serve,
// which binds loopback and keeps its store in the workdir.
func checkLocalFlags() error {
	for name, value := range map[string]string{
		"bind": flagBind, "data-dir": flagDataDir, "gcs-bucket": flagGCSBucket,
		"keys-file": flagKeysFile,
	} {
		if value != "" {
			return fmt.Errorf("--%s is for --shared; a local serve binds loopback "+
				"and keeps its store in the workdir", name)
		}
	}
	if flagBackend != moondb.DefaultBackend {
		return fmt.Errorf("--backend %s is for --shared; a local serve keeps its store "+
			"in the workdir", flagBackend)
	}
	if flagReplicateTo != "" && flagReplicateKeyFile == "" {
		return errors.New("--replicate-to needs --replicate-key-file (or " +
			"MOONDB_REPLICATE_KEY_FILE): the file holding the instance's key")
	}
	return nil
}

// checkSharedFlags wants what a shared instance cannot guess: where to
// listen, which keys to accept, and for the cloud backend which bucket.
func checkSharedFlags() error {
	switch {
	case flagBind == "":
		return errors.New("shared mode needs an explicit --bind host:port (or MOONDB_BIND): " +
			"0.0.0.0:7433 inside a container, 127.0.0.1:7433 to try it on this machine")
	case flagKeysFile == "":
		return errors.New("shared mode needs --keys-file (or MOONDB_KEYS_FILE): " +
			"the hex SHA-256 of each key it accepts, one per line")
	case flagReplicateTo != "":
		return errors.New("--replicate-to is for a local serve; " +
			"a shared instance is where records land")
	}
	if _, _, err := net.SplitHostPort(flagBind); err != nil {
		return fmt.Errorf("--bind %q: want host:port: %w", flagBind, err)
	}
	switch flagBackend {
	case "local":
		return nil
	case "cloud":
		if flagGCSBucket == "" {
			return errors.New("--backend cloud needs --gcs-bucket (or MOONDB_GCS_BUCKET)")
		}
		return nil
	}
	return fmt.Errorf("unknown --backend %q: want local or cloud", flagBackend)
}
