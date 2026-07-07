package scanner

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Rule is a compiled detection rule with metadata.
type Rule struct {
	ID          string
	Description string
	Pattern     *regexp.Regexp
	Severity    string // "high", "medium", "low"

	// MinEntropy, when > 0, requires the secret (capture group 1 if present,
	// otherwise the full match) to have at least this Shannon entropy in
	// bits per character. Filters out placeholders and repeated characters.
	MinEntropy float64

	// ExcludeFiles lists basename globs (filepath.Match syntax) where this
	// rule does not apply, e.g. lockfiles full of high-entropy hashes.
	ExcludeFiles []string

	// SkipOverlapping drops a match whose span is contained within a match
	// already reported by an earlier rule on the same line. Used by broad
	// rules that would otherwise duplicate specific ones.
	SkipOverlapping bool

	// Filter, when set, can veto a match. It receives the secret (capture
	// group 1 if present, otherwise the full match); returning false drops it.
	Filter func(secret string) bool
}

var DefaultRules = []Rule{
	{
		ID:          "aws-access-key-id",
		Description: "AWS access key ID",
		Pattern:     regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16})`),
		Severity:    "high",
	},
	{
		ID:          "aws-secret-access-key",
		Description: "AWS secret access key",
		Pattern: regexp.MustCompile(
			`(?i)aws[_\-\s]*secret[_\-\s]*(?:access[_\-\s]*)?key[\s]*[=:]["']?\s*([A-Za-z0-9/+]{40})`,
		),
		Severity: "high",
	},
	{
		ID:          "github-pat-classic",
		Description: "GitHub token (PAT, OAuth, user-to-server, server-to-server, refresh)",
		Pattern:     regexp.MustCompile(`(gh[pousr]_[A-Za-z0-9]{36,255})`),
		Severity:    "high",
	},
	{
		ID:          "github-pat-fine-grained",
		Description: "GitHub fine-grained personal access token",
		Pattern:     regexp.MustCompile(`(github_pat_[A-Za-z0-9_]{82,255})`),
		Severity:    "high",
	},
	{
		ID:          "gitlab-pat",
		Description: "GitLab personal access token",
		Pattern:     regexp.MustCompile(`(glpat-[A-Za-z0-9\-_]{20,64})`),
		Severity:    "high",
	},
	{
		ID:          "google-api-key",
		Description: "Google API key",
		Pattern:     regexp.MustCompile(`(AIza[0-9A-Za-z\-_]{35})`),
		Severity:    "high",
	},
	{
		ID:          "google-oauth-client-secret",
		Description: "Google OAuth client secret",
		Pattern: regexp.MustCompile(
			`(?i)(?:google|gcp|oauth)[_\-\s]*(?:client[_\-\s]*)?secret[\s]*[=:]["'\s]*([A-Za-z0-9\-_]{24,64})`,
		),
		Severity: "high",
	},
	{
		ID:          "slack-bot-token",
		Description: "Slack bot token",
		Pattern:     regexp.MustCompile(`(xoxb-[0-9A-Za-z\-]{16,255})`),
		Severity:    "high",
	},
	{
		ID:          "slack-user-token",
		Description: "Slack user token",
		Pattern:     regexp.MustCompile(`(xoxp-[0-9A-Za-z\-]{16,255})`),
		Severity:    "high",
	},
	{
		ID:          "slack-app-token",
		Description: "Slack app/workspace/service tokens",
		Pattern:     regexp.MustCompile(`(xox[oas]-[0-9A-Za-z\-]{16,255})`),
		Severity:    "high",
	},
	{
		ID:          "slack-app-level-token",
		Description: "Slack app-level token",
		Pattern:     regexp.MustCompile(`(xapp-[0-9]-[A-Za-z0-9]+-[0-9]+-[a-f0-9]{32,})`),
		Severity:    "high",
	},
	{
		ID:          "slack-webhook",
		Description: "Slack incoming webhook URL",
		Pattern: regexp.MustCompile(
			`(https://hooks\.slack\.com/services/T[A-Z0-9]+/B[A-Z0-9]+/[A-Za-z0-9]+)`,
		),
		Severity: "high",
	},
	{
		ID:          "private-key-header",
		Description: "PEM private key block (RSA, EC, DSA, OPENSSH, PGP, encrypted PKCS#8, SSH2)",
		Pattern:     regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`),
		Severity:    "high",
	},
	{
		ID:          "putty-private-key",
		Description: "PuTTY private key file (PPK)",
		Pattern:     regexp.MustCompile(`PuTTY-User-Key-File-[0-9]`),
		Severity:    "high",
	},
	{
		ID:          "pem-certificate-with-key",
		Description: "PEM certificate block that may contain private material",
		Pattern:     regexp.MustCompile(`-----BEGIN CERTIFICATE-----`),
		Severity:    "low",
	},
	{
		ID:          "jwt-token",
		Description: "JSON Web Token",
		// eyJ... base64url header, then two more base64url segments separated by dots.
		Pattern:  regexp.MustCompile(`(eyJ[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+)`),
		Severity: "medium",
	},
	{
		ID:          "generic-secret-assignment",
		Description: "Generic high-entropy secret assignment in code",
		Pattern: regexp.MustCompile(
			`(?i)(?:password|passwd|secret|token|api[_\-]?key|auth[_\-]?key|access[_\-]?key)[\s]*[=:]\s*["']([^"'\s]{8,}?)["']`,
		),
		Severity:   "medium",
		MinEntropy: 3.0,
	},
	{
		ID:          "unquoted-secret-assignment",
		Description: "Unquoted secret assignment (.env / YAML style)",
		Pattern: regexp.MustCompile(
			`(?i)(?:password|passwd|pwd|secret|token|api[_\-]?key|auth[_\-]?key|access[_\-]?key|client[_\-]?secret|private[_\-]?key)\s*[=:]\s*([A-Za-z0-9_\-+/=.@#!%^&*]{8,})(?:[\s;,)]|$)`,
		),
		Severity:   "medium",
		MinEntropy: 3.3,
	},
	{
		ID:          "db-connection-string",
		Description: "Database connection string containing credentials",
		Pattern: regexp.MustCompile(
			`(?i)((?:postgres|postgresql|mysql|mongodb(?:\+srv)?|redis)://[^:\s]+:[^@\s]+@[^\s"']+)`,
		),
		Severity: "high",
	},
	{
		ID:          "heroku-api-key",
		Description: "Heroku API key",
		Pattern: regexp.MustCompile(
			`(?i)heroku[_\-\s]*(?:api[_\-\s]*)?(?:key|token)[\s]*[=:]["'\s]*([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`,
		),
		Severity: "high",
	},
	{
		ID:          "twilio-api-key",
		Description: "Twilio API key or account SID",
		Pattern:     regexp.MustCompile(`(SK[0-9a-fA-F]{32}|AC[0-9a-fA-F]{32})`),
		Severity:    "high",
	},
	{
		ID:          "sendgrid-api-key",
		Description: "SendGrid API key",
		Pattern:     regexp.MustCompile(`(SG\.[A-Za-z0-9\-_]{22,66}\.[A-Za-z0-9\-_]{43,66})`),
		Severity:    "high",
	},
	{
		ID:          "stripe-live-secret-key",
		Description: "Stripe live secret key",
		Pattern:     regexp.MustCompile(`(sk_live_[A-Za-z0-9]{24,99})`),
		Severity:    "high",
	},
	{
		ID:          "stripe-live-restricted-key",
		Description: "Stripe live restricted key",
		Pattern:     regexp.MustCompile(`(rk_live_[A-Za-z0-9]{24,99})`),
		Severity:    "high",
	},
	{
		ID:          "npm-token",
		Description: "npm access token",
		Pattern:     regexp.MustCompile(`(npm_[A-Za-z0-9]{36,255})`),
		Severity:    "high",
	},
	{
		ID:          "pypi-token",
		Description: "PyPI upload token",
		Pattern:     regexp.MustCompile(`(pypi-[A-Za-z0-9\-_]{32,255})`),
		Severity:    "high",
	},
	{
		ID:          "nuget-api-key",
		Description: "NuGet API key",
		Pattern:     regexp.MustCompile(`(oy2[A-Za-z0-9]{43})`),
		Severity:    "high",
	},
	{
		ID:          "dockerhub-token",
		Description: "Docker Hub personal access token",
		Pattern:     regexp.MustCompile(`(dckr_pat_[A-Za-z0-9\-_]{27,255})`),
		Severity:    "high",
	},

	// AI/ML provider keys — highest growth category for leaked secrets
	{
		ID:          "openai-api-key",
		Description: "OpenAI API key (legacy)",
		Pattern:     regexp.MustCompile(`(sk-[a-zA-Z0-9]{20}T3BlbkFJ[a-zA-Z0-9]{20})`),
		Severity:    "high",
	},
	{
		ID:          "openai-project-key",
		Description: "OpenAI project API key",
		Pattern:     regexp.MustCompile(`(sk-proj-[a-zA-Z0-9_\-]{82,255})`),
		Severity:    "high",
	},
	{
		ID:          "anthropic-api-key",
		Description: "Anthropic API key",
		Pattern:     regexp.MustCompile(`(sk-ant-(?:api03|admin01)-[a-zA-Z0-9_\-]{90,110})`),
		Severity:    "high",
	},
	{
		ID:          "huggingface-token",
		Description: "HuggingFace access token",
		Pattern:     regexp.MustCompile(`(hf_[a-zA-Z0-9]{34,})`),
		Severity:    "high",
	},

	// Cloud providers
	{
		ID:          "digitalocean-pat",
		Description: "DigitalOcean personal access token",
		Pattern:     regexp.MustCompile(`(dop_v1_[a-f0-9]{64})`),
		Severity:    "high",
	},
	{
		ID:          "cloudflare-api-token",
		Description: "Cloudflare API token",
		Pattern: regexp.MustCompile(
			`(?i)(?:cloudflare|cf)[_\-\s]*(?:api[_\-\s]*)?token[\s]*[=:]["'\s]*([a-zA-Z0-9_\-]{40})`,
		),
		Severity: "high",
	},
	{
		ID:          "azure-client-secret",
		Description: "Azure client secret",
		Pattern: regexp.MustCompile(
			`(?i)azure[_\-\s]*client[_\-\s]*secret[\s]*[=:]["'\s]*([a-zA-Z0-9~._\-]{34,40})`,
		),
		Severity: "high",
	},

	// Observability and identity
	{
		ID:          "datadog-api-key",
		Description: "Datadog API key",
		Pattern: regexp.MustCompile(
			`(?i)(?:datadog|dd)[_\-\s]*api[_\-\s]*key[\s]*[=:]["'\s]*([a-f0-9]{32})`,
		),
		Severity: "high",
	},
	{
		ID:          "okta-api-token",
		Description: "Okta API token",
		Pattern: regexp.MustCompile(
			`(?i)(?:okta[_\-\s]*(?:api)?[_\-\s]*token|ssws|org[_\-\s]*api[_\-\s]*token)["'\s]*[=:\s]["'\s]*(00[A-Za-z0-9_\-]{38,42})`,
		),
		Severity: "medium",
	},
	{
		ID:          "vercel-token",
		Description: "Vercel API token",
		Pattern:     regexp.MustCompile(`(vcp_[a-zA-Z0-9]{24,})`),
		Severity:    "high",
	},

	// Platform tokens
	{
		ID:          "linear-api-key",
		Description: "Linear API key",
		Pattern:     regexp.MustCompile(`(lin_api_[a-zA-Z0-9]{40})`),
		Severity:    "high",
	},
	{
		ID:          "shopify-access-token",
		Description: "Shopify access token",
		Pattern:     regexp.MustCompile(`(shp(?:at|ca|ss)_[a-f0-9]{32})`),
		Severity:    "high",
	},
	{
		ID:          "planetscale-token",
		Description: "Planetscale service token",
		Pattern:     regexp.MustCompile(`(pscale_tkn_[a-zA-Z0-9_\-]{32,})`),
		Severity:    "high",
	},

	// AI/ML providers — continued
	{
		ID:          "cohere-api-key",
		Description: "Cohere API key",
		Pattern: regexp.MustCompile(
			`(?i)cohere[_\-\s]*(?:api[_\-\s]*)?key[\s]*[=:]["'\s]*([a-zA-Z0-9]{40,})`,
		),
		Severity: "high",
	},
	{
		ID:          "replicate-api-token",
		Description: "Replicate API token",
		Pattern:     regexp.MustCompile(`(r8_[a-zA-Z0-9]{20,})`),
		Severity:    "high",
	},

	// Cloud — GCP service account JSON and AWS session tokens
	{
		ID:          "gcp-service-account-json",
		Description: "GCP service account key (JSON)",
		Pattern:     regexp.MustCompile(`"type"\s*:\s*"service_account"`),
		Severity:    "high",
	},
	{
		ID:          "aws-session-token",
		Description: "AWS temporary session token",
		Pattern:     regexp.MustCompile(`(ASIA[0-9A-Z]{16})`),
		Severity:    "high",
	},

	// Git platforms
	{
		ID:          "bitbucket-app-password",
		Description: "Bitbucket app password or API token",
		Pattern: regexp.MustCompile(
			`(?i)bitbucket[_\-\s]*(?:app[_\-\s]*)?(?:password|token|secret)[\s]*[=:]["'\s]*([A-Za-z0-9]{18,64})`,
		),
		Severity: "high",
	},
	{
		ID:          "azure-devops-pat",
		Description: "Azure DevOps personal access token",
		Pattern: regexp.MustCompile(
			`(?i)(?:azure[_\-\s]*devops|ado|vsts)[_\-\s]*(?:pat|token)[\s]*[=:]["'\s]*([a-z2-7]{52})`,
		),
		Severity: "high",
	},

	// CI/CD
	{
		ID:          "circleci-token",
		Description: "CircleCI personal API token",
		Pattern: regexp.MustCompile(
			`(?i)(?:circle[_\-\s]*ci|circleci)[_\-\s]*token[\s]*[=:]["'\s]*([a-f0-9]{40})`,
		),
		Severity: "high",
	},

	// Messaging
	{
		ID:          "discord-bot-token",
		Description: "Discord bot token",
		// First segment is a base64-encoded snowflake ID, which always starts M, N, or O.
		Pattern: regexp.MustCompile(
			`([MNO][A-Za-z0-9]{23,27}\.[A-Za-z0-9_\-]{6}\.[A-Za-z0-9_\-]{27,40})`,
		),
		Severity: "high",
	},
	{
		ID:          "telegram-bot-token",
		Description: "Telegram bot token",
		Pattern:     regexp.MustCompile(`([0-9]{8,10}:[A-Za-z0-9_\-]{35})`),
		Severity:    "high",
	},

	// Auth and identity
	{
		ID:          "firebase-web-api-key",
		Description: "Firebase web API key",
		Pattern: regexp.MustCompile(
			`(?i)firebase[_\-\s]*(?:api[_\-\s]*)?key[\s]*[=:]["'\s]*(AIza[0-9A-Za-z\-_]{35})`,
		),
		Severity: "medium",
	},
	{
		ID:          "supabase-service-key",
		Description: "Supabase service role key",
		Pattern:     regexp.MustCompile(`(sbp_[a-f0-9]{40,})`),
		Severity:    "high",
	},

	// Infrastructure
	{
		ID:          "terraform-cloud-token",
		Description: "Terraform Cloud / Enterprise API token",
		Pattern:     regexp.MustCompile(`(atlasv1-[a-zA-Z0-9\-_]{60,})`),
		Severity:    "high",
	},
	{
		ID:          "hashicorp-vault-token",
		Description: "HashiCorp Vault token",
		Pattern:     regexp.MustCompile(`(hvs\.[a-zA-Z0-9_\-]{24,})`),
		Severity:    "high",
	},

	// Email services
	{
		ID:          "mailgun-api-key",
		Description: "Mailgun API key",
		Pattern: regexp.MustCompile(
			`(?i)mailgun[_\-\s]*(?:api[_\-\s]*)?(?:key|token)[\s]*[=:]["'\s]*(key-[a-f0-9]{32})`,
		),
		Severity: "high",
	},
	{
		ID:          "mailchimp-api-key",
		Description: "Mailchimp API key",
		Pattern:     regexp.MustCompile(`([a-f0-9]{32}-us[0-9]{1,2})`),
		Severity:    "high",
	},

	// Cross-tool validated rules (detected by GitLeaks + GitHub Secret Scanning + TruffleHog)
	{
		ID:          "databricks-pat",
		Description: "Databricks personal access token",
		Pattern:     regexp.MustCompile(`\b(dapi[a-f0-9]{32})\b`),
		Severity:    "high",
	},
	{
		ID:          "grafana-service-account-token",
		Description: "Grafana service account token",
		Pattern:     regexp.MustCompile(`(glsa_[A-Za-z0-9]{32}_[A-Fa-f0-9]{8})`),
		Severity:    "high",
	},
	{
		ID:          "grafana-cloud-api-token",
		Description: "Grafana Cloud API token",
		Pattern:     regexp.MustCompile(`(glc_[A-Za-z0-9+/]{32,}={0,2})`),
		Severity:    "high",
	},
	{
		ID:          "newrelic-user-api-key",
		Description: "New Relic user API key",
		Pattern:     regexp.MustCompile(`(NRAK-[A-Z0-9]{27})`),
		Severity:    "high",
	},
	{
		ID:          "newrelic-insert-key",
		Description: "New Relic insert/browser key",
		Pattern:     regexp.MustCompile(`(NRI[IJS]-[A-Za-z0-9]{27,32})`),
		Severity:    "high",
	},
	{
		ID:          "postman-api-token",
		Description: "Postman API token",
		Pattern:     regexp.MustCompile(`(PMAK-[a-f0-9]{24}-[a-f0-9]{34})`),
		Severity:    "high",
	},
	{
		ID:          "doppler-api-token",
		Description: "Doppler API token",
		Pattern:     regexp.MustCompile(`(dp\.pt\.[a-zA-Z0-9]{43})`),
		Severity:    "high",
	},
	{
		ID:          "sentry-org-token",
		Description: "Sentry organization auth token",
		Pattern:     regexp.MustCompile(`(sntrys_eyJ[A-Za-z0-9+/]{50,})`),
		Severity:    "high",
	},
	{
		ID:          "sentry-user-token",
		Description: "Sentry user auth token",
		Pattern:     regexp.MustCompile(`(sntryu_[a-f0-9]{64})`),
		Severity:    "high",
	},
	{
		ID:          "atlassian-api-token",
		Description: "Atlassian API token (Jira/Confluence)",
		Pattern:     regexp.MustCompile(`(ATATT3[A-Za-z0-9_\-=]{100,})`),
		Severity:    "high",
	},
	{
		ID:          "pulumi-api-token",
		Description: "Pulumi API token",
		Pattern:     regexp.MustCompile(`(pul-[a-f0-9]{40})`),
		Severity:    "high",
	},
	{
		ID:          "flyio-access-token",
		Description: "Fly.io access token",
		Pattern:     regexp.MustCompile(`(fo1_[A-Za-z0-9_\-]{43})`),
		Severity:    "high",
	},
	{
		ID:          "mapbox-api-token",
		Description: "Mapbox public API token",
		Pattern:     regexp.MustCompile(`(pk\.[a-zA-Z0-9]{60}\.[a-zA-Z0-9]{22})`),
		Severity:    "medium",
	},
	{
		ID:          "perplexity-api-key",
		Description: "Perplexity AI API key",
		Pattern:     regexp.MustCompile(`(pplx-[a-zA-Z0-9]{48})`),
		Severity:    "high",
	},
	{
		ID:          "rubygems-api-token",
		Description: "RubyGems API token",
		Pattern:     regexp.MustCompile(`(rubygems_[a-f0-9]{48})`),
		Severity:    "high",
	},
	{
		ID:          "alibaba-access-key-id",
		Description: "Alibaba Cloud access key ID",
		Pattern:     regexp.MustCompile(`\b(LTAI[A-Za-z0-9]{20})\b`),
		Severity:    "high",
	},

	// Source code patterns — catch tokens in function calls, headers, and string literals
	{
		ID:          "authorization-bearer",
		Description: "Authorization header with Bearer token",
		Pattern: regexp.MustCompile(
			`(?i)(?:authorization|auth)\s*[:=]\s*["']?Bearer\s+([A-Za-z0-9_\-\.]{20,})`,
		),
		Severity: "high",
	},
	{
		ID:          "hardcoded-password-string",
		Description: "Hardcoded password in string literal",
		Pattern: regexp.MustCompile(
			`(?i)(?:password|passwd|pwd)\s*[:=]\s*` + "`" + `?"([^"'\` + "`" + `\s]{8,})"?`,
		),
		Severity:   "medium",
		MinEntropy: 3.0,
	},

	// Payments and webhooks
	{
		ID:          "stripe-webhook-secret",
		Description: "Stripe webhook signing secret",
		Pattern:     regexp.MustCompile(`(whsec_[A-Za-z0-9]{32,64})`),
		Severity:    "high",
	},

	// Git platforms — continued
	{
		ID:          "gitlab-runner-token",
		Description: "GitLab runner authentication token",
		Pattern:     regexp.MustCompile(`(glrt-[A-Za-z0-9_\-]{20,64})`),
		Severity:    "high",
	},
	{
		ID:          "gitlab-deploy-token",
		Description: "GitLab deploy token",
		Pattern:     regexp.MustCompile(`(gldt-[A-Za-z0-9_\-]{20,64})`),
		Severity:    "high",
	},

	// AI/ML providers — continued
	{
		ID:          "groq-api-key",
		Description: "Groq API key",
		Pattern:     regexp.MustCompile(`(gsk_[A-Za-z0-9]{52})`),
		Severity:    "high",
	},

	// Infrastructure — continued
	{
		ID:          "tailscale-key",
		Description: "Tailscale auth/API/client key",
		Pattern:     regexp.MustCompile(`(tskey-(?:auth|api|client)-[A-Za-z0-9\-]{12,})`),
		Severity:    "high",
	},
	{
		ID:          "dynatrace-api-token",
		Description: "Dynatrace API token",
		Pattern:     regexp.MustCompile(`(dt0c01\.[A-Z0-9]{24}\.[A-Z0-9]{64})`),
		Severity:    "high",
	},
	{
		ID:          "age-secret-key",
		Description: "age encryption secret key",
		Pattern:     regexp.MustCompile(`(AGE-SECRET-KEY-1[A-Z0-9]{58})`),
		Severity:    "high",
	},
	{
		ID:          "azure-storage-account-key",
		Description: "Azure storage account key",
		Pattern: regexp.MustCompile(
			`(?i)(?:account[_\-\s]*key|storage[_\-\s]*key)[\s]*[=:;]["'\s]*([A-Za-z0-9+/]{86}==)`,
		),
		Severity: "high",
	},

	// Generic catch-alls — kept last so SkipOverlapping can defer to specific rules
	{
		ID:          "url-basic-auth",
		Description: "Credentials embedded in a URL",
		Pattern: regexp.MustCompile(
			`(?i)\b[a-z][a-z0-9+.\-]{1,20}://[^/\s:@'"$<{}]{2,64}:[^/\s:@'"$<{}]{2,64}@[^\s'"]{2,}`,
		),
		Severity: "medium",

		SkipOverlapping: true,
	},
	{
		ID:          "high-entropy-string",
		Description: "High-entropy string (possible secret)",
		Pattern:     regexp.MustCompile(`([A-Za-z0-9+/=_\-]{40,})`),
		Severity:    "low",
		MinEntropy:  4.7,
		ExcludeFiles: []string{
			"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lockb",
			"go.sum", "Cargo.lock", "composer.lock", "Gemfile.lock", "*.lock",
			"*.min.js", "*.map", "*.svg", "*.ipynb",
		},
		SkipOverlapping: true,
		Filter:          notKnownHashOrPublicKey,
	},
}

// notKnownHashOrPublicKey vetoes high-entropy matches that are well-known
// non-secret blobs: subresource-integrity hashes and SSH public key material.
func notKnownHashOrPublicKey(secret string) bool {
	for _, prefix := range []string{"sha256-", "sha384-", "sha512-", "AAAAB3NzaC1", "AAAAC3NzaC1"} {
		if strings.HasPrefix(secret, prefix) {
			return false
		}
	}
	return true
}

// FileRule flags a staged file as sensitive based on its name alone,
// regardless of content. This catches binary key material (PKCS#12, JKS)
// that content rules can never see, and credential files like .env.
type FileRule struct {
	ID          string
	Description string
	Patterns    []string // basename globs, filepath.Match syntax
	Excludes    []string // basename globs that negate a pattern match
	Severity    string
}

// Matches reports whether base matches one of the rule's patterns and none
// of its excludes.
func (fr FileRule) Matches(base string) bool {
	for _, e := range fr.Excludes {
		if ok, _ := filepath.Match(e, base); ok {
			return false
		}
	}
	for _, p := range fr.Patterns {
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
	}
	return false
}

var DefaultFileRules = []FileRule{
	{
		ID:          "ssh-private-key-file",
		Description: "SSH private key file",
		Patterns:    []string{"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519"},
		Severity:    "high",
	},
	{
		ID:          "key-material-file",
		Description: "Key or keystore file",
		Patterns:    []string{"*.key", "*.p12", "*.pfx", "*.jks", "*.keystore", "*.ppk", "*.p8"},
		Severity:    "high",
	},
	{
		ID:          "pem-file",
		Description: "PEM file (may contain private key material)",
		Patterns:    []string{"*.pem"},
		Severity:    "medium",
	},
	{
		ID:          "dotenv-file",
		Description: "Environment file with potential credentials",
		Patterns:    []string{".env", ".env.*"},
		Excludes:    []string{".env.example", ".env.sample", ".env.template", ".env.dist"},
		Severity:    "medium",
	},
	{
		ID:          "credential-store-file",
		Description: "Credential store file",
		Patterns:    []string{".netrc", "_netrc", ".git-credentials", ".htpasswd", ".pypirc"},
		Severity:    "high",
	},
	{
		ID:          "terraform-state-file",
		Description: "Terraform state file (often contains secrets)",
		Patterns:    []string{"*.tfstate", "*.tfstate.backup"},
		Severity:    "high",
	},
	{
		ID:          "kubeconfig-file",
		Description: "Kubernetes config file with cluster credentials",
		Patterns:    []string{"kubeconfig"},
		Severity:    "high",
	},
}
