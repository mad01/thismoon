package present

import (
	"fmt"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultWorkdir is the page store directory a fresh install uses when
// PRESENT_WORKDIR is unset. The leading ~ is expanded at runtime by the CLI,
// never at build time.
const DefaultWorkdir = "~/.local/state/present"

// LegacyWorkdir is where the page store lived before it moved to the XDG
// state directory. An install that has published pages there keeps using
// it — pages are not migrated, so a machine that has them must not be
// pointed somewhere else. Every other machine lands in DefaultWorkdir. The
// fleet recipe and the MCP's seatbelt profile both name this path
// explicitly, so a supervised process never depends on which branch the
// resolution takes.
const LegacyWorkdir = "~/.config/present"

// LegacyWorkdirProbe is the artifact that proves pages live in
// LegacyWorkdir. The directory alone proves nothing: fleet provisioning
// puts the render template and assets there on every machine, so its
// existence would pin even a machine that has never published a page.
const LegacyWorkdirProbe = "pages"

// DefaultPort is the port `present serve` listens on and tool URLs point at
// when PRESENT_PORT is unset.
const DefaultPort = 7423

// DefaultBind is the interface `present serve` listens on when PRESENT_BIND
// is unset: loopback, because local mode authenticates nothing. A shared
// instance has to name its bind explicitly, so exposure is always a stated
// choice.
const DefaultBind = "127.0.0.1"

// DefaultSpeakURL is the speak service the page view registers a page's text
// with for read-aloud when PRESENT_SPEAK_URL is unset: the fleet's local
// speak behind the .this front door. An empty value turns read-aloud off,
// which is what a shared instance sets, since no speak runs beside it.
const DefaultSpeakURL = "http://speak.this"

// SharedTTL is how long an ephemeral page on a shared instance lives after
// it is created or shared again.
const SharedTTL = 30 * 24 * time.Hour

// DefaultStore is the page store `present serve` opens when PRESENT_STORE is
// unset: the filesystem under the workdir. A shared instance in Kubernetes
// passes k8s.
const DefaultStore = "fs"

// DefaultSweepInterval is how often a shared instance on the Kubernetes
// store purges expired ephemeral pages. Expired pages already read as
// missing between sweeps, so the interval only bounds how long their
// objects linger.
const DefaultSweepInterval = 10 * time.Minute

// MaxPageBytes caps one page on a shared instance. The cluster store keeps a
// page as a single object and the object store caps those a little above
// this; a page that would not fit is refused at the API instead of failing
// deep inside a write.
const MaxPageBytes = 1 << 20

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	baseURL := fmt.Sprintf("http://localhost:%d", DefaultPort)
	return agentdoc.Facts{
		Name:          "present",
		Bin:           "present",
		Purpose:       "scrollable briefing pages agents publish and keep editing, served on localhost",
		BaseURL:       baseURL,
		StorePath:     DefaultWorkdir,
		HasDoctor:     true,
		MCPDoctorTool: "present_doctor",
		MCPNote:       "Tools read and write the page store directly; present serve (default " + baseURL + ") only serves the page URLs the tools return.",
	}
}

// SharedFacts returns the facts a shared instance renders into its MCP
// instructions and error hints. Same binary, but the tools run inside the
// serving process and the store is the instance's own, so the local
// two-process story does not apply.
func SharedFacts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:          "present",
		Bin:           "present",
		Purpose:       "shared briefing pages: agents create and edit them here over HTTP, readers reach them by link",
		HasDoctor:     true,
		MCPDoctorTool: "present_doctor",
		MCPNote: "Tools write this instance's page store directly and return URLs on this instance. " +
			"A page is reachable by its id alone; present_update needs the author key this connection sends as its bearer token.",
	}
}
