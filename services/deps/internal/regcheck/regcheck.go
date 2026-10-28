// Package regcheck looks each pinned version up in the registry that publishes
// it: the npm registry, PyPI, and the Go module proxy. A registry can pull or
// flag a version before any advisory exists, or without one: npm withdraws a
// compromised release, PyPI removes a release or quarantines a malware project,
// and the Go proxy refuses to serve a module it considers malicious. Those come
// back as malicious findings. A yanked, deprecated, archived or retracted
// version comes back as a signal. Each finding's id is one of the rule ids
// below.
//
// A lookup the registry did not answer comes back as an unknown Result, never
// as a clean one. The scanner then keeps the findings the version had before.
package regcheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/module"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// Rule ids, one per thing a registry can say about a version. Each is the id
// of a store.Advisory: a deps- prefix and no ':', since the FlagKey parser
// splits on the last colon.
const (
	// RuleNPMWithdrawn: npm still lists the version's publish time but no
	// longer serves it. Malicious.
	RuleNPMWithdrawn = "deps-npm-withdrawn"
	// RuleNPMDeprecated: the version carries an npm deprecation notice. Signal.
	RuleNPMDeprecated = "deps-npm-deprecated"
	// RulePyPIWithdrawn: PyPI lists the project but no longer serves the
	// version. Malicious.
	RulePyPIWithdrawn = "deps-pypi-withdrawn"
	// RulePyPIQuarantined: PyPI quarantined the project (PEP 792), which it
	// does to a project reported as malware. Malicious.
	RulePyPIQuarantined = "deps-pypi-quarantined"
	// RulePyPIYanked: the release is yanked. Signal.
	RulePyPIYanked = "deps-pypi-yanked"
	// RulePyPIArchived: the project is archived (PEP 792) and expects no more
	// releases, security fixes included. Signal.
	RulePyPIArchived = "deps-pypi-archived"
	// RulePyPIDeprecated: the project is deprecated (PEP 792). Signal.
	RulePyPIDeprecated = "deps-pypi-deprecated"
	// RuleGoBlocked: the Go module proxy refuses to serve the module because it
	// considers it malicious. Malicious.
	RuleGoBlocked = "deps-go-blocked"
	// RuleGoRetracted: the go.mod of the module's highest version retracts the
	// version. Signal.
	RuleGoRetracted = "deps-go-retracted"
)

// ruleKinds maps every rule id to the kind its findings carry. It is also the
// set IsRule checks.
var ruleKinds = map[string]store.Kind{
	RuleNPMWithdrawn:    store.KindMalicious,
	RuleNPMDeprecated:   store.KindSignal,
	RulePyPIWithdrawn:   store.KindMalicious,
	RulePyPIQuarantined: store.KindMalicious,
	RulePyPIYanked:      store.KindSignal,
	RulePyPIArchived:    store.KindSignal,
	RulePyPIDeprecated:  store.KindSignal,
	RuleGoBlocked:       store.KindMalicious,
	RuleGoRetracted:     store.KindSignal,
}

// IsRule reports whether id is one of this package's rule ids, which tells a
// registry finding apart from an OSV advisory or another producer's rule.
func IsRule(id string) bool {
	_, ok := ruleKinds[id]
	return ok
}

// Query names one package version to look up. OtherIndex marks a version
// whose manifest also names another package source (Dependency.OtherIndex):
// the registry not serving it is then no evidence it was withdrawn.
type Query struct {
	Ecosystem  string
	Name       string
	Version    string
	OtherIndex bool
}

// Result is the registry's answer for one query. Err is set when the registry
// gave no usable answer (a network error, a timeout, an unexpected status); the
// version's registry state is then unknown and Findings is nil. A zero Result
// means the registry had nothing to say about the version.
type Result struct {
	Findings []store.Advisory
	Err      error
}

// Endpoints are the registry base URLs a Client queries.
type Endpoints struct {
	NPM     string
	PyPI    string
	GoProxy string
}

// publicEndpoints are the public registries New queries.
var publicEndpoints = Endpoints{
	NPM:     "https://registry.npmjs.org",
	PyPI:    "https://pypi.org",
	GoProxy: "https://proxy.golang.org",
}

const (
	// maxInFlight caps the packages looked up at once in one Check. Each
	// package's requests run one after another, so this caps the requests in
	// flight too.
	maxInFlight = 8
	// requestTimeout bounds one registry request.
	requestTimeout = 10 * time.Second
	// passBudget bounds one whole Check. A registry that hangs costs a check at
	// most this long, and the lookups still waiting come back unknown. A full
	// check runs discovery, OSV and this pass inside the CLI's 2-minute
	// request timeout.
	passBudget = 45 * time.Second
	// maxBody caps one response. The largest deps reads is a full npm package
	// document, about 16 MB for typescript.
	maxBody = 64 << 20
	// userAgent names deps to the registries.
	userAgent = "thismoon-deps (registry check)"
)

// Client queries the package registries.
type Client struct {
	endpoints Endpoints
	http      *http.Client
	// private returns the go command's GONOPROXY and GOPRIVATE patterns, comma
	// separated. The public Go proxy is never asked about a module that
	// matches either.
	private func(ctx context.Context) (string, error)
}

// New returns a Client against the public registries, which reads GONOPROXY
// and GOPRIVATE from the go command on every Check.
func New() *Client {
	c := NewWithEndpoints(publicEndpoints)
	c.private = goEnvPrivate
	return c
}

// NewWithEndpoints returns a Client against e, with no private module patterns
// (used by tests with an httptest server).
func NewWithEndpoints(e Endpoints) *Client {
	return &Client{
		endpoints: e,
		http:      &http.Client{Timeout: requestTimeout},
		private:   func(context.Context) (string, error) { return "", nil },
	}
}

// pkgKey names one package in one registry.
type pkgKey struct {
	ecosystem, name string
}

// Check looks every query up in its registry and returns one Result per query,
// aligned by index (queries[i] → result[i]). Each package is looked up once
// however many queries name it, at most maxInFlight packages at a time, and
// the whole pass stops after passBudget. Ecosystems with no registry here
// (SwiftURL, GitHub Actions) get a zero Result.
func (c *Client) Check(ctx context.Context, queries []Query) []Result {
	ctx, cancel := context.WithTimeout(ctx, passBudget)
	defer cancel()

	var order []pkgKey
	versions := map[pkgKey][]string{}
	for _, q := range queries {
		k := pkgKey{q.Ecosystem, q.Name}
		if _, ok := versions[k]; !ok {
			order = append(order, k)
		}
		if !slices.Contains(versions[k], q.Version) {
			versions[k] = append(versions[k], q.Version)
		}
	}
	private, privateErr := c.private(ctx)

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		answers = make(map[pkgKey]map[string]Result, len(order))
		slots   = make(chan struct{}, maxInFlight)
	)
	for _, k := range order {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			got := c.lookup(ctx, k, versions[k], private, privateErr)
			mu.Lock()
			answers[k] = got
			mu.Unlock()
		})
	}
	wg.Wait()

	out := make([]Result, len(queries))
	for i, q := range queries {
		r := answers[pkgKey{q.Ecosystem, q.Name}][q.Version]
		if q.OtherIndex {
			r.Findings = withoutPyPIWithdrawn(r.Findings)
		}
		out[i] = r
	}
	return out
}

// lookup answers every version of one package from its registry. A nil map
// means the registry has nothing to say about any of them.
func (c *Client) lookup(
	ctx context.Context,
	k pkgKey,
	versions []string,
	private string,
	privateErr error,
) map[string]Result {
	switch k.ecosystem {
	case "npm":
		return c.npm(ctx, k.name, versions)
	case "PyPI":
		return c.pypi(ctx, k.name, versions)
	case "Go":
		if privateErr != nil {
			return allUnknown(versions, privateErr)
		}
		if module.MatchPrefixPatterns(private, k.name) {
			return nil
		}
		return c.goModule(ctx, k.name, versions)
	}
	return nil
}

// goEnvPrivate asks the go command for GONOPROXY and GOPRIVATE and returns
// both pattern lists as one. GONOPROXY names the modules go fetches straight
// from their origin. GOPRIVATE still counts when GONOPROXY is "none": go then
// fetches private modules through a company GOPROXY, and the public proxy
// must not hear their names either.
func goEnvPrivate(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "go", "env", "GONOPROXY", "GOPRIVATE").Output()
	if err != nil {
		return "", fmt.Errorf("go env GONOPROXY GOPRIVATE: %w", err)
	}
	return strings.Join(strings.Fields(string(out)), ","), nil
}

// get fetches url and returns its status and body. Any status is an answer for
// the caller to read; only a request that got no response, or a body that
// could not be read in full, is an error.
func (c *Client) get(ctx context.Context, url, accept string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return 0, nil, fmt.Errorf("read %s: %w", url, err)
	}
	if len(body) > maxBody {
		return 0, nil, fmt.Errorf("read %s: response over %d bytes", url, maxBody)
	}
	return res.StatusCode, body, nil
}

// statusErr reports a status the caller has no reading for.
func statusErr(url string, status int) error {
	return fmt.Errorf("GET %s: unexpected status %d", url, status)
}

// finding builds the finding for rule, with the kind the rule carries.
func finding(rule, summary string) store.Advisory {
	return store.Advisory{ID: rule, Kind: ruleKinds[rule], Summary: summary}
}

// found is a Result holding the given findings.
func found(fs ...store.Advisory) Result {
	return Result{Findings: fs}
}

// allUnknown answers every version with err.
func allUnknown(versions []string, err error) map[string]Result {
	out := make(map[string]Result, len(versions))
	for _, v := range versions {
		out[v] = Result{Err: err}
	}
	return out
}

// allFound answers every version with the same finding.
func allFound(versions []string, f store.Advisory) map[string]Result {
	out := make(map[string]Result, len(versions))
	for _, v := range versions {
		out[v] = found(f)
	}
	return out
}
