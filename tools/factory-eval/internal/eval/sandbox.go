package eval

// Sandbox is the sandbox block of a run's settings file. It puts Claude
// Code's OS sandbox (seatbelt on macOS) around every Bash command of the
// arm session, the factory's workers included, since a subagent shares its
// parent's sandbox. The permission lists stay the first layer and decide
// which commands run; the sandbox decides what a running command can reach,
// so it holds when an allowed command does more than its name says, such as
// a test binary that writes outside the clone. A --settings file outranks
// the user's settings, so the block applies on a machine whose own settings
// leave the sandbox off.
type Sandbox struct {
	Enabled bool `json:"enabled"`
	// AutoAllowBashIfSandboxed stays false: the lists keep deciding which
	// commands run, and dontAsk keeps denying the rest.
	AutoAllowBashIfSandboxed bool `json:"autoAllowBashIfSandboxed"`
	// AllowUnsandboxedCommands stays false: no command may retry outside
	// the sandbox after it blocked one.
	AllowUnsandboxedCommands bool `json:"allowUnsandboxedCommands"`
	// FailIfUnavailable makes claude exit at startup when the sandbox
	// cannot start, rather than run the arm unsandboxed.
	FailIfUnavailable bool `json:"failIfUnavailable"`
	// ExcludedCommands is empty on purpose. The user's settings may take
	// commands such as git out of their own sandbox; a run must not
	// inherit that, and the first probe showed git sandboxed with this
	// empty list in the file.
	ExcludedCommands []string          `json:"excludedCommands"`
	Network          SandboxNetwork    `json:"network"`
	Filesystem       SandboxFilesystem `json:"filesystem"`
}

// SandboxNetwork closes the network. No domain is allowed, and
// StrictAllowlist denies any other host instead of prompting, which dontAsk
// would deny anyway. AllowLocalBinding stays on because the components'
// tests start httptest servers on a loopback port, which the sandbox
// otherwise refuses with "bind: operation not permitted"; the price is that
// a test binary can also reach the machine's own localhost services.
// AllowAllUnixSockets is written as false so a run never inherits a true
// from the user's settings.
type SandboxNetwork struct {
	StrictAllowlist     bool `json:"strictAllowlist"`
	AllowLocalBinding   bool `json:"allowLocalBinding"`
	AllowAllUnixSockets bool `json:"allowAllUnixSockets"`
}

// SandboxFilesystem names the writable paths beyond the working directory,
// the sandbox's own temp dir and the --add-dir dir.
type SandboxFilesystem struct {
	AllowWrite []string `json:"allowWrite"`
}

// armSandbox is the block for a run whose temp dir is root. Writes stay
// inside root (the clone, its worktrees and the settings file) and the Go
// build cache, which `go test` and `make test` need. The source checkout
// and the user's code tree are then closed at the OS level, not only by
// the deny rules. An empty goCache adds no cache path.
func armSandbox(root, goCache string) Sandbox {
	write := []string{root}
	if goCache != "" {
		write = append(write, goCache)
	}
	return Sandbox{
		Enabled:           true,
		FailIfUnavailable: true,
		ExcludedCommands:  []string{},
		Network:           SandboxNetwork{StrictAllowlist: true, AllowLocalBinding: true},
		Filesystem:        SandboxFilesystem{AllowWrite: write},
	}
}
