package export

import (
	"bytes"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// cdnBase is the public npm CDN an export loads the browser libraries from.
// It serves each file of an npm package byte for byte, which is what lets
// an integrity hash computed from the asset cache match what it sends.
const cdnBase = "https://cdn.jsdelivr.net/npm/"

// library is one browser library the page shell loads from the asset cache
// (scripts/cache-assets.sh, docs/adr/0022): the cached file under
// <workdir>/assets/js and the same bytes at an npm path on the CDN.
type library struct {
	name string // what the page needs it for; also its key in a needs set
	file string // the cached file name
	npm  string // <package>@<version>/<path> on the CDN
}

// Cached files the CDN serves from its npm mirror. The order is the load
// order the shell uses: a plugin after what it extends.
var libraries = []library{
	{"cytoscape", "cytoscape-3.31.0.min.js", "cytoscape@3.31.0/dist/cytoscape.min.js"},
	{"dagre", "dagre-0.8.5.min.js", "dagre@0.8.5/dist/dagre.min.js"},
	{"cytoscape-dagre", "cytoscape-dagre-2.5.0.js", "cytoscape-dagre@2.5.0/cytoscape-dagre.js"},
	{"elkjs", "elk-0.12.0.bundled.js", "elkjs@0.12.0/lib/elk.bundled.js"},
	{"cytoscape-elk", "cytoscape-elk-2.3.0.js", "cytoscape-elk@2.3.0/dist/cytoscape-elk.js"},
	// The cache holds jsDelivr's generated chart.umd.min.js, which is its
	// banner comment and then the npm file chart.umd.js byte for byte. The
	// export strips the banner and loads the npm file, whose bytes are fixed.
	{"chart.js", "chart-4.4.6.umd.min.js", "chart.js@4.4.6/dist/chart.umd.js"},
	{
		"chartjs-chart-sankey",
		"chartjs-chart-sankey-0.15.3.min.js",
		"chartjs-chart-sankey@0.15.3/dist/chartjs-chart-sankey.min.js",
	},
	{"d3", "d3-7.9.0.min.js", "d3@7.9.0/dist/d3.min.js"},
}

// The renderer's markup for the blocks that need a library.
var (
	reGraphBlock = regexp.MustCompile(`id="cy-graph"`)
	reChartSpec  = regexp.MustCompile(
		`(?s)<script type="application/json" class="chart-spec">(.*?)</script>`,
	)
	reDiagram     = regexp.MustCompile(`class="diagram-spec"`)
	reELKEngine   = regexp.MustCompile(`['"]elk(?:-[a-z]+)?['"]`)
	reChartCall   = regexp.MustCompile(`\bnew Chart\(`)
	generatedHead = []byte("/**\n * Skipped minification")
	commentEnd    = []byte("*/\n")
)

// needs returns the libraries a rendition's body and the page's graph
// script use. The graph needs cytoscape and dagre, the base engine its
// control falls back to, plus ELK when the script names an ELK engine. A
// chart needs Chart.js, a sankey its plugin too, and a ribbon d3 alone. A
// diagram needs ELK and d3. A terminal needs nothing: an export cannot run
// one. A legacy graph given as raw JS runs whatever it holds, so one that
// builds a chart itself needs Chart.js too.
func needs(body, graph string) map[string]bool {
	set := map[string]bool{}
	if graph != "" && reGraphBlock.MatchString(body) {
		addAll(set, "cytoscape", "dagre", "cytoscape-dagre")
		if reELKEngine.MatchString(graph) {
			addAll(set, "elkjs", "cytoscape-elk")
		}
	}
	if reChartCall.MatchString(graph) {
		addAll(set, "chart.js")
	}
	for _, m := range reChartSpec.FindAllStringSubmatch(body, -1) {
		var spec struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(
			[]byte(m[1]),
			&spec,
		) // a spec that does not parse is drawn as a bar chart
		switch spec.Kind {
		case "ribbon":
			addAll(set, "d3")
		case "sankey":
			addAll(set, "chart.js", "chartjs-chart-sankey")
		default:
			addAll(set, "chart.js")
		}
	}
	if reDiagram.MatchString(body) {
		addAll(set, "elkjs", "d3")
	}
	return set
}

func addAll(set map[string]bool, names ...string) {
	for _, n := range names {
		set[n] = true
	}
}

// cdnScript is one library tag in the exported file: the CDN URL and the
// integrity hash the browser checks the response against.
type cdnScript struct {
	Package   string // <package>@<version>, for the notice at the head of the file
	URL       string
	Integrity string
}

// cdnScripts resolves every needed library, in load order, to its CDN URL
// and the integrity hash of the cached file. It fails when a needed file is
// not in the asset cache, since the hash cannot be known without it.
func cdnScripts(assets string, set map[string]bool) ([]cdnScript, error) {
	var out []cdnScript
	for _, lib := range libraries {
		if !set[lib.name] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(assets, "js", lib.file))
		if err != nil {
			return nil, fmt.Errorf(
				"export: %s is not in the asset cache, run make cache in services/present: %w",
				lib.name, err)
		}
		out = append(out, cdnScript{
			Package:   strings.SplitN(lib.npm, "/", 2)[0],
			URL:       cdnBase + lib.npm,
			Integrity: integrity(npmBytes(data)),
		})
	}
	return out, nil
}

// npmBytes is a cached file as its npm package ships it: jsDelivr's banner
// on a file it generated is cut off, anything else is returned unchanged.
func npmBytes(data []byte) []byte {
	if !bytes.HasPrefix(data, generatedHead) {
		return data
	}
	end := bytes.Index(data, commentEnd)
	if end < 0 {
		return data
	}
	return data[end+len(commentEnd):]
}

// integrity is the Subresource Integrity value for data.
func integrity(data []byte) string {
	sum := sha512.Sum384(data)
	return "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
}
