package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/services/d-man/internal/tlsca"
)

// systemKeychain is where a machine-wide trusted root belongs; trusting a cert
// there is what makes every browser and curl accept d-man's minted leaves.
const systemKeychain = "/Library/Keychains/System.keychain"

var flagCADir string

var caCmd = &cobra.Command{
	Use:   "ca",
	Short: "Manage d-man's local TLS certificate authority",
	Long: `The ca subcommands manage the local certificate authority d-man uses to serve
blocked HTTPS hosts with a trusted certificate.

Install it once (writing the system keychain needs root):
  sudo d-man ca install

After that, d-man serve can present a valid cert for any blocked host, so a
blocked HTTPS site opens the local block page instead of a certificate warning.`,
}

var caInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Generate the CA if needed and trust it in the system keychain",
	RunE:  runCAInstall,
}

var caUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the d-man CA from the system keychain",
	RunE:  runCAUninstall,
}

var caPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the CA directory",
	RunE:  runCAPath,
}

func init() {
	caCmd.PersistentFlags().StringVar(&flagCADir, "ca-dir", "",
		"directory holding the CA cert/key (default: <config dir>/ca)")
	caCmd.AddCommand(caInstallCmd, caUninstallCmd, caPathCmd)
	rootCmd.AddCommand(caCmd)
}

// resolveCADir returns the CA directory: the --ca-dir flag if set, else a "ca"
// directory beside the routes config so the CA travels with the config. A
// ~-prefixed --ca-dir that cannot be expanded is an error rather than a
// directory relative to the working directory — the CA key minted there
// signs certificates the whole system trusts.
func resolveCADir() (string, error) {
	if flagCADir != "" {
		return confdir.Expand(flagCADir)
	}
	return filepath.Join(filepath.Dir(flagConfig), "ca"), nil
}

func runCAInstall(cmd *cobra.Command, _ []string) error {
	dir, err := resolveCADir()
	if err != nil {
		return err
	}
	if _, err := tlsca.EnsureCA(dir); err != nil {
		return err
	}
	certPath := tlsca.CertPath(dir)
	// -d: add to the admin (system) domain; -r trustRoot: trust for SSL as a
	// root; -k: the keychain to add it to.
	c := exec.Command("security", "add-trusted-cert", "-d",
		"-r", "trustRoot", "-k", systemKeychain, certPath)
	c.Stdout, c.Stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		if os.Geteuid() != 0 {
			return fmt.Errorf("trusting the CA in the system keychain needs root — try: sudo d-man ca install")
		}
		return fmt.Errorf("add-trusted-cert: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "installed and trusted %q (%s)\n", tlsca.CommonName, certPath)
	return nil
}

func runCAUninstall(cmd *cobra.Command, _ []string) error {
	c := exec.Command("security", "delete-certificate", "-c", tlsca.CommonName, systemKeychain)
	c.Stdout, c.Stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		if os.Geteuid() != 0 {
			return fmt.Errorf("removing the CA from the system keychain needs root — try: sudo d-man ca uninstall")
		}
		return fmt.Errorf("delete-certificate: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "removed %q from %s\n", tlsca.CommonName, systemKeychain)
	return nil
}

func runCAPath(cmd *cobra.Command, _ []string) error {
	dir, err := resolveCADir()
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), dir)
	return nil
}
