package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Redact
// ---------------------------------------------------------------------------

func TestRedact(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "one char",
			input: "x",
			want:  "*",
		},
		{
			name:  "two chars",
			input: "ab",
			want:  "**",
		},
		{
			name:  "three chars",
			input: "abc",
			want:  "ab*",
		},
		{
			name:  "medium 8 chars",
			input: "abcdefgh",
			want:  "ab******",
		},
		{
			name:  "medium 12 chars",
			input: "abcdefghijkl",
			want:  "ab**********",
		},
		{
			name:  "long 13 chars - boundary",
			input: "abcdefghijklm",
			want:  "abcd*****jklm",
		},
		{
			name:  "long 20 chars",
			input: "abcdefghijklmnopqrst",
			want:  "abcd************qrst",
		},
		{
			name:  "long 40 chars",
			input: "AKIAIOSFODNN7EXAMPLE1234567890abcdefghij",
			want:  "AKIA********************************ghij",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Redact(tc.input)
			if got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DefaultRules pattern matching
// ---------------------------------------------------------------------------

func TestDefaultRulesDetection(t *testing.T) {
	cases := []struct {
		name      string
		ruleID    string
		input     string
		wantMatch bool
	}{
		// AWS access key ID
		{
			name:      "aws-access-key-id matches",
			ruleID:    "aws-access-key-id",
			input:     "AKIA1234567890ABCDEF",
			wantMatch: true,
		},
		{
			name:      "aws-access-key-id no match on normal code",
			ruleID:    "aws-access-key-id",
			input:     "func main() {}",
			wantMatch: false,
		},

		// GitHub PAT classic — pattern requires 36+ alphanumeric chars after the prefix
		{
			name:      "github-pat-classic ghp_ matches",
			ruleID:    "github-pat-classic",
			input:     "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			wantMatch: true,
		},
		{
			name:      "github-pat-classic gho_ matches",
			ruleID:    "github-pat-classic",
			input:     "gho_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			wantMatch: true,
		},
		{
			name:      "github-pat-classic no match on variable name",
			ruleID:    "github-pat-classic",
			input:     "var githubToken = \"\"",
			wantMatch: false,
		},

		// GitHub fine-grained PAT
		{
			name:      "github-pat-fine-grained matches",
			ruleID:    "github-pat-fine-grained",
			input:     "github_pat_" + strings.Repeat("A", 82),
			wantMatch: true,
		},
		{
			name:      "github-pat-fine-grained too short no match",
			ruleID:    "github-pat-fine-grained",
			input:     "github_pat_short",
			wantMatch: false,
		},

		// Slack bot token
		{
			name:      "slack-bot-token matches",
			ruleID:    "slack-bot-token",
			input:     "xoxb-123456789-abcdefgh",
			wantMatch: true,
		},
		{
			name:      "slack-bot-token no match on comment",
			ruleID:    "slack-bot-token",
			input:     "// set SLACK_TOKEN env var",
			wantMatch: false,
		},

		// Slack user token
		{
			name:      "slack-user-token matches",
			ruleID:    "slack-user-token",
			input:     "xoxp-987654321-zyxwvuts",
			wantMatch: true,
		},

		// Slack webhook
		{
			name:      "slack-webhook matches",
			ruleID:    "slack-webhook",
			input:     "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
			wantMatch: true,
		},
		{
			name:      "slack-webhook no match on unrelated URL",
			ruleID:    "slack-webhook",
			input:     "https://example.com/webhook",
			wantMatch: false,
		},

		// Private key header
		{
			name:      "private-key-header RSA matches",
			ruleID:    "private-key-header",
			input:     "-----BEGIN RSA PRIVATE KEY-----",
			wantMatch: true,
		},
		{
			name:      "private-key-header EC matches",
			ruleID:    "private-key-header",
			input:     "-----BEGIN EC PRIVATE KEY-----",
			wantMatch: true,
		},
		{
			name:      "private-key-header OPENSSH matches",
			ruleID:    "private-key-header",
			input:     "-----BEGIN OPENSSH PRIVATE KEY-----",
			wantMatch: true,
		},
		{
			name:      "private-key-header no match on public key",
			ruleID:    "private-key-header",
			input:     "-----BEGIN PUBLIC KEY-----",
			wantMatch: false,
		},

		// PEM certificate
		{
			name:      "pem-certificate-with-key matches",
			ruleID:    "pem-certificate-with-key",
			input:     "-----BEGIN CERTIFICATE-----",
			wantMatch: true,
		},

		// JWT token
		{
			name:      "jwt-token matches",
			ruleID:    "jwt-token",
			input:     "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123",
			wantMatch: true,
		},
		{
			name:      "jwt-token no match on random dots",
			ruleID:    "jwt-token",
			input:     "version = 1.2.3",
			wantMatch: false,
		},

		// Generic secret assignment — pattern matches keyword[\s]*[=:]["']value["'] (no space between = and quote)
		{
			name:      "generic-secret-assignment password matches",
			ruleID:    "generic-secret-assignment",
			input:     `password="mysecretpassword123"`,
			wantMatch: true,
		},
		{
			name:      "generic-secret-assignment token matches",
			ruleID:    "generic-secret-assignment",
			input:     `token="abcdefghijklmn"`,
			wantMatch: true,
		},
		{
			name:      "generic-secret-assignment no match empty value",
			ruleID:    "generic-secret-assignment",
			input:     `password=""`,
			wantMatch: false,
		},
		// gofmt-idiomatic spaced assignments: whitespace between `=`/`:` and
		// the opening quote must not evade the rule.
		{
			name:      "generic-secret-assignment token spaced assignment matches",
			ruleID:    "generic-secret-assignment",
			input:     `token = "Xk9Lm2Qp4Rz8Nv3Wb7Yd"`,
			wantMatch: true,
		},
		{
			name:      "generic-secret-assignment secret spaced assignment matches",
			ruleID:    "generic-secret-assignment",
			input:     `secret = "Qp4Rz8Nv3Wb7Yd6Xk9Lm"`,
			wantMatch: true,
		},
		{
			name:      "generic-secret-assignment api_key single-quoted spaced assignment matches",
			ruleID:    "generic-secret-assignment",
			input:     `api_key = 'Nv3Wb7Yd6Xk9Lm2Qp4Rz8'`,
			wantMatch: true,
		},
		{
			name:      "generic-secret-assignment colon spaced assignment matches",
			ruleID:    "generic-secret-assignment",
			input:     `access_key: "Wb7Yd6Xk9Lm2Qp4Rz8Nv3"`,
			wantMatch: true,
		},

		// DB connection string
		{
			name:      "db-connection-string postgres matches",
			ruleID:    "db-connection-string",
			input:     "postgres://user:pass@host:5432/db",
			wantMatch: true,
		},
		{
			name:      "db-connection-string mysql matches",
			ruleID:    "db-connection-string",
			input:     "mysql://admin:secret@localhost:3306/mydb",
			wantMatch: true,
		},
		{
			name:      "db-connection-string mongodb matches",
			ruleID:    "db-connection-string",
			input:     "mongodb://user:pass@mongo:27017/mydb",
			wantMatch: true,
		},
		{
			name:      "db-connection-string no match plain string",
			ruleID:    "db-connection-string",
			input:     "DATABASE_HOST=localhost",
			wantMatch: false,
		},

		// Stripe live secret key
		{
			name:      "stripe-live-secret-key matches",
			ruleID:    "stripe-live-secret-key",
			input:     "sk_live_1234567890abcdefghijklmn",
			wantMatch: true,
		},
		{
			name:      "stripe-live-secret-key no match on test key prefix",
			ruleID:    "stripe-live-secret-key",
			input:     "sk_test_1234567890abcdefghijklmn",
			wantMatch: false,
		},

		// Google API key
		{
			name:      "google-api-key matches",
			ruleID:    "google-api-key",
			input:     "AIzaSyDdI0hCZtE6vySjMm-WEfRq3CPzqKqqsHI",
			wantMatch: true,
		},
		{
			name:      "google-api-key no match on short string",
			ruleID:    "google-api-key",
			input:     "AIza",
			wantMatch: false,
		},

		// GitLab PAT
		{
			name:      "gitlab-pat matches",
			ruleID:    "gitlab-pat",
			input:     "glpat-abcdefghij12345678901",
			wantMatch: true,
		},

		// npm token
		{
			name:      "npm-token matches",
			ruleID:    "npm-token",
			input:     "npm_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			wantMatch: true,
		},

		// Stripe restricted key
		{
			name:      "stripe-live-restricted-key matches",
			ruleID:    "stripe-live-restricted-key",
			input:     "rk_live_1234567890abcdefghijklmn",
			wantMatch: true,
		},

		// AWS secret access key
		{
			name:      "aws-secret-access-key matches",
			ruleID:    "aws-secret-access-key",
			input:     `aws_secret_key="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"`,
			wantMatch: true,
		},
		{
			name:      "aws-secret-access-key no match without keyword",
			ruleID:    "aws-secret-access-key",
			input:     `key="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"`,
			wantMatch: false,
		},

		// Google OAuth client secret
		{
			name:      "google-oauth-client-secret matches",
			ruleID:    "google-oauth-client-secret",
			input:     `google_client_secret="AbCdEfGhIjKlMnOpQrStUvWx"`,
			wantMatch: true,
		},
		{
			name:      "google-oauth-client-secret no match on unrelated key",
			ruleID:    "google-oauth-client-secret",
			input:     `stripe_secret="AbCdEfGhIjKlMnOpQrStUvWx"`,
			wantMatch: false,
		},

		// Slack app/workspace/service token
		{
			name:      "slack-app-token xoxa matches",
			ruleID:    "slack-app-token",
			input:     "xoxa-1234567890abcdef",
			wantMatch: true,
		},
		{
			name:      "slack-app-token xoxo matches",
			ruleID:    "slack-app-token",
			input:     "xoxo-1234567890abcdef",
			wantMatch: true,
		},
		{
			name:      "slack-app-token xoxs matches",
			ruleID:    "slack-app-token",
			input:     "xoxs-1234567890abcdef",
			wantMatch: true,
		},
		{
			name:      "slack-app-token no match on xoxb prefix",
			ruleID:    "slack-app-token",
			input:     "xoxb-1234567890abcdef",
			wantMatch: false,
		},

		// Heroku API key
		{
			name:      "heroku-api-key matches",
			ruleID:    "heroku-api-key",
			input:     `heroku_api_key="a1b2c3d4-e5f6-7890-abcd-ef1234567890"`,
			wantMatch: true,
		},
		{
			name:      "heroku-api-key no match without heroku keyword",
			ruleID:    "heroku-api-key",
			input:     `api_key="a1b2c3d4-e5f6-7890-abcd-ef1234567890"`,
			wantMatch: false,
		},

		// Twilio API key
		{
			name:      "twilio-api-key SK prefix matches",
			ruleID:    "twilio-api-key",
			input:     "SK1234567890abcdef1234567890abcdef12",
			wantMatch: true,
		},
		{
			name:      "twilio-api-key AC prefix matches",
			ruleID:    "twilio-api-key",
			input:     "AC1234567890abcdef1234567890abcdef12",
			wantMatch: true,
		},
		{
			name:      "twilio-api-key no match on short value",
			ruleID:    "twilio-api-key",
			input:     "SK1234",
			wantMatch: false,
		},

		// SendGrid API key
		{
			name:      "sendgrid-api-key matches",
			ruleID:    "sendgrid-api-key",
			input:     "SG.ABCDEFGHIJKLMNOPQRSTUVwx." + strings.Repeat("A", 43),
			wantMatch: true,
		},
		{
			name:      "sendgrid-api-key no match without SG prefix",
			ruleID:    "sendgrid-api-key",
			input:     "XX.ABCDEFGHIJKLMNOPQRSTUVwx.ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn",
			wantMatch: false,
		},

		// PyPI token
		{
			name:      "pypi-token matches",
			ruleID:    "pypi-token",
			input:     "pypi-AgEIcHlwaS5vcmcCJDExMTExMTExLTExMTEtMTExMS0xMTEx",
			wantMatch: true,
		},
		{
			name:      "pypi-token no match on short value",
			ruleID:    "pypi-token",
			input:     "pypi-short",
			wantMatch: false,
		},

		// NuGet API key
		{
			name:      "nuget-api-key matches",
			ruleID:    "nuget-api-key",
			input:     "oy2" + strings.Repeat("a", 43),
			wantMatch: true,
		},
		{
			name:      "nuget-api-key no match with wrong prefix",
			ruleID:    "nuget-api-key",
			input:     "oy3abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNO",
			wantMatch: false,
		},

		// Docker Hub token
		{
			name:      "dockerhub-token matches",
			ruleID:    "dockerhub-token",
			input:     "dckr_pat_" + strings.Repeat("a", 27),
			wantMatch: true,
		},
		{
			name:      "dockerhub-token no match with wrong prefix",
			ruleID:    "dockerhub-token",
			input:     "docker_pat_abcdefghijklmnopqrstuvwxy",
			wantMatch: false,
		},

		// OpenAI API key (legacy)
		{
			name:      "openai-api-key matches",
			ruleID:    "openai-api-key",
			input:     "sk-" + strings.Repeat("A", 20) + "T3BlbkFJ" + strings.Repeat("b", 20),
			wantMatch: true,
		},
		{
			name:      "openai-api-key no match without T3BlbkFJ marker",
			ruleID:    "openai-api-key",
			input:     "sk-ABCDEFGHIJKLMNOPQRSTabcdefghijklmnopqrstu",
			wantMatch: false,
		},

		// OpenAI project key
		{
			name:      "openai-project-key matches",
			ruleID:    "openai-project-key",
			input:     "sk-proj-" + strings.Repeat("A", 82),
			wantMatch: true,
		},
		{
			name:      "openai-project-key no match on short value",
			ruleID:    "openai-project-key",
			input:     "sk-proj-short",
			wantMatch: false,
		},

		// Anthropic API key
		{
			name:      "anthropic-api-key api03 matches",
			ruleID:    "anthropic-api-key",
			input:     "sk-ant-api03-" + strings.Repeat("A", 90),
			wantMatch: true,
		},
		{
			name:      "anthropic-api-key admin01 matches",
			ruleID:    "anthropic-api-key",
			input:     "sk-ant-admin01-" + strings.Repeat("B", 90),
			wantMatch: true,
		},
		{
			name:      "anthropic-api-key no match with wrong variant",
			ruleID:    "anthropic-api-key",
			input:     "sk-ant-v1-" + strings.Repeat("A", 90),
			wantMatch: false,
		},

		// HuggingFace token
		{
			name:      "huggingface-token matches",
			ruleID:    "huggingface-token",
			input:     "hf_abcdefghijklmnopqrstuvwxyzABCDEFGH",
			wantMatch: true,
		},
		{
			name:      "huggingface-token no match on short value",
			ruleID:    "huggingface-token",
			input:     "hf_short",
			wantMatch: false,
		},

		// DigitalOcean PAT
		{
			name:      "digitalocean-pat matches",
			ruleID:    "digitalocean-pat",
			input:     "dop_v1_" + strings.Repeat("a", 64),
			wantMatch: true,
		},
		{
			name:      "digitalocean-pat no match with wrong prefix",
			ruleID:    "digitalocean-pat",
			input:     "do_v1_" + strings.Repeat("a", 64),
			wantMatch: false,
		},

		// Cloudflare API token
		{
			name:      "cloudflare-api-token matches",
			ruleID:    "cloudflare-api-token",
			input:     `cloudflare_api_token="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"`,
			wantMatch: true,
		},
		{
			name:      "cloudflare-api-token no match without cloudflare keyword",
			ruleID:    "cloudflare-api-token",
			input:     `api_token="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"`,
			wantMatch: false,
		},

		// Azure client secret
		{
			name:      "azure-client-secret matches",
			ruleID:    "azure-client-secret",
			input:     "azure_client_secret=\"" + strings.Repeat("A", 34) + "\"",
			wantMatch: true,
		},
		{
			name:      "azure-client-secret no match without azure keyword",
			ruleID:    "azure-client-secret",
			input:     `client_secret="AbCdEfGhIjKlMnOpQrStUvWxYzABCDEFG"`,
			wantMatch: false,
		},

		// Datadog API key
		{
			name:      "datadog-api-key matches",
			ruleID:    "datadog-api-key",
			input:     `datadog_api_key="abcdef1234567890abcdef1234567890"`,
			wantMatch: true,
		},
		{
			name:      "datadog-api-key no match without datadog keyword",
			ruleID:    "datadog-api-key",
			input:     `api_key="abcdef1234567890abcdef1234567890"`,
			wantMatch: false,
		},

		// Okta API token
		{
			name:      "okta-api-token matches env var",
			ruleID:    "okta-api-token",
			input:     "OKTA_API_TOKEN=00abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN",
			wantMatch: true,
		},
		{
			name:      "okta-api-token matches SSWS header",
			ruleID:    "okta-api-token",
			input:     `Authorization: SSWS 00abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN`,
			wantMatch: true,
		},
		{
			name:      "okta-api-token matches json value",
			ruleID:    "okta-api-token",
			input:     `"org_api_token": "00abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"`,
			wantMatch: true,
		},
		{
			name:      "okta-api-token no match bare hex (false positive fix)",
			ruleID:    "okta-api-token",
			input:     "00abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN",
			wantMatch: false,
		},
		{
			name:      "okta-api-token no match pypi url hash",
			ruleID:    "okta-api-token",
			input:     "packages/9c/a6/49e40067abcdef1234567890abcdef1234567890ea29e10b5a9b1ba783/flytekit-1.16.15.whl",
			wantMatch: false,
		},
		{
			name:      "okta-api-token no match git sha",
			ruleID:    "okta-api-token",
			input:     "commit 00abcdefabcdefabcdefabcdefabcdefabcdefab",
			wantMatch: false,
		},

		// Vercel token
		{
			name:      "vercel-token matches",
			ruleID:    "vercel-token",
			input:     "vcp_abcdefghijklmnopqrstuvwxy",
			wantMatch: true,
		},
		{
			name:      "vercel-token no match with wrong prefix",
			ruleID:    "vercel-token",
			input:     "vcx_abcdefghijklmnopqrstuvwxy",
			wantMatch: false,
		},

		// Linear API key
		{
			name:      "linear-api-key matches",
			ruleID:    "linear-api-key",
			input:     "lin_api_" + strings.Repeat("a", 40),
			wantMatch: true,
		},
		{
			name:      "linear-api-key no match with wrong prefix",
			ruleID:    "linear-api-key",
			input:     "lin_key_abcdefghijklmnopqrstuvwxyzABCDEFGHIJ",
			wantMatch: false,
		},

		// Shopify access token
		{
			name:      "shopify-access-token shpat matches",
			ruleID:    "shopify-access-token",
			input:     "shpat_" + strings.Repeat("a", 32),
			wantMatch: true,
		},
		{
			name:      "shopify-access-token shpca matches",
			ruleID:    "shopify-access-token",
			input:     "shpca_" + strings.Repeat("b", 32),
			wantMatch: true,
		},
		{
			name:      "shopify-access-token shpss matches",
			ruleID:    "shopify-access-token",
			input:     "shpss_" + strings.Repeat("c", 32),
			wantMatch: true,
		},
		{
			name:      "shopify-access-token no match with wrong prefix",
			ruleID:    "shopify-access-token",
			input:     "shpzz_" + strings.Repeat("a", 32),
			wantMatch: false,
		},

		// PlanetScale token
		{
			name:      "planetscale-token matches",
			ruleID:    "planetscale-token",
			input:     "pscale_tkn_" + strings.Repeat("a", 32),
			wantMatch: true,
		},
		{
			name:      "planetscale-token no match with wrong prefix",
			ruleID:    "planetscale-token",
			input:     "pscale_key_abcdefghijklmnopqrstuvwxyzAB",
			wantMatch: false,
		},

		// Cohere API key — keyword-gated to avoid matching arbitrary co- strings
		{
			name:      "cohere-api-key matches",
			ruleID:    "cohere-api-key",
			input:     `cohere_api_key="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"`,
			wantMatch: true,
		},
		{
			name:      "cohere-api-key no match without cohere keyword",
			ruleID:    "cohere-api-key",
			input:     `api_key="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"`,
			wantMatch: false,
		},

		// Replicate API token
		{
			name:      "replicate-api-token matches",
			ruleID:    "replicate-api-token",
			input:     "r8_abcdefghijklmnopqrstu",
			wantMatch: true,
		},
		{
			name:      "replicate-api-token no match with wrong prefix",
			ruleID:    "replicate-api-token",
			input:     "r9_abcdefghijklmnopqrstu",
			wantMatch: false,
		},

		// GCP service account JSON
		{
			name:      "gcp-service-account-json matches",
			ruleID:    "gcp-service-account-json",
			input:     `"type" : "service_account"`,
			wantMatch: true,
		},
		{
			name:      "gcp-service-account-json no match on wrong type",
			ruleID:    "gcp-service-account-json",
			input:     `"type": "user_account"`,
			wantMatch: false,
		},

		// AWS session token
		{
			name:      "aws-session-token matches",
			ruleID:    "aws-session-token",
			input:     "ASIA1234567890ABCDEF",
			wantMatch: true,
		},
		{
			name:      "aws-session-token no match on AKIA prefix",
			ruleID:    "aws-session-token",
			input:     "AKIA1234567890ABCDEF",
			wantMatch: false,
		},

		// Bitbucket app password
		{
			name:      "bitbucket-app-password matches",
			ruleID:    "bitbucket-app-password",
			input:     `bitbucket_app_password="AbcDefGhiJklMnoPqrS"`,
			wantMatch: true,
		},
		{
			name:      "bitbucket-app-password no match without bitbucket keyword",
			ruleID:    "bitbucket-app-password",
			input:     `app_password="AbcDefGhiJklMnoPqrS"`,
			wantMatch: false,
		},

		// Azure DevOps PAT — [a-z2-7]{52} is base32; lowercase letters and digits 2-7
		{
			name:      "azure-devops-pat matches",
			ruleID:    "azure-devops-pat",
			input:     "ado_pat=\"" + strings.Repeat("a", 52) + "\"",
			wantMatch: true,
		},
		{
			name:      "azure-devops-pat no match without ado keyword",
			ruleID:    "azure-devops-pat",
			input:     `pat="abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwx"`,
			wantMatch: false,
		},

		// CircleCI token
		{
			name:      "circleci-token matches",
			ruleID:    "circleci-token",
			input:     `circleci_token="abcdef1234567890abcdef1234567890abcdef12"`,
			wantMatch: true,
		},
		{
			name:      "circleci-token no match without circleci keyword",
			ruleID:    "circleci-token",
			input:     `ci_token="abcdef1234567890abcdef1234567890abcdef12"`,
			wantMatch: false,
		},

		// Discord bot token — 24-28 alphanum, dot, 6 alphanum/dash/underscore, dot, 27-40 alphanum/dash/underscore
		{
			name:      "discord-bot-token matches",
			ruleID:    "discord-bot-token",
			input:     "MTIzNDU2Nzg5MDEyMzQ1Njc4.abcdef.ABCDEFGHIJKLMNOPQRSTUVWXYZAB",
			wantMatch: true,
		},
		{
			name:      "discord-bot-token no match on short segment",
			ruleID:    "discord-bot-token",
			input:     "short.ab.ABCDEFGHIJKLMNOPQRSTUVWXYZAB",
			wantMatch: false,
		},

		// Telegram bot token — 8-10 digits, colon, 35 alphanum/dash/underscore
		{
			name:      "telegram-bot-token matches",
			ruleID:    "telegram-bot-token",
			input:     "12345678:" + strings.Repeat("A", 35),
			wantMatch: true,
		},
		{
			name:      "telegram-bot-token no match on short number",
			ruleID:    "telegram-bot-token",
			input:     "1234:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefg",
			wantMatch: false,
		},

		// Firebase web API key
		{
			name:      "firebase-web-api-key matches",
			ruleID:    "firebase-web-api-key",
			input:     `firebase_api_key="AIzaSyDdI0hCZtE6vySjMm-WEfRq3CPzqKqqsHI"`,
			wantMatch: true,
		},
		{
			name:      "firebase-web-api-key no match without firebase keyword",
			ruleID:    "firebase-web-api-key",
			input:     `api_key="AIzaSyDdI0hCZtE6vySjMm-WEfRq3CPzqKqqsHI"`,
			wantMatch: false,
		},

		// Supabase service key
		{
			name:      "supabase-service-key matches",
			ruleID:    "supabase-service-key",
			input:     "sbp_" + strings.Repeat("a", 40),
			wantMatch: true,
		},
		{
			name:      "supabase-service-key no match with wrong prefix",
			ruleID:    "supabase-service-key",
			input:     "sbs_" + strings.Repeat("a", 40),
			wantMatch: false,
		},

		// Terraform Cloud token
		{
			name:      "terraform-cloud-token matches",
			ruleID:    "terraform-cloud-token",
			input:     "atlasv1-" + strings.Repeat("a", 60),
			wantMatch: true,
		},
		{
			name:      "terraform-cloud-token no match with wrong prefix",
			ruleID:    "terraform-cloud-token",
			input:     "atlasv2-" + strings.Repeat("a", 60),
			wantMatch: false,
		},

		// HashiCorp Vault token
		{
			name:      "hashicorp-vault-token matches",
			ruleID:    "hashicorp-vault-token",
			input:     "hvs.abcdefghijklmnopqrstuvwxyz",
			wantMatch: true,
		},
		{
			name:      "hashicorp-vault-token no match with wrong prefix",
			ruleID:    "hashicorp-vault-token",
			input:     "hvb.abcdefghijklmnopqrstuvwxyz",
			wantMatch: false,
		},

		// Mailgun API key — keyword-gated; bare key-<hex32> collides with cache keys etc.
		{
			name:      "mailgun-api-key matches",
			ruleID:    "mailgun-api-key",
			input:     `mailgun_api_key="key-abcdef1234567890abcdef1234567890"`,
			wantMatch: true,
		},
		{
			name:      "mailgun-api-key no match without mailgun keyword",
			ruleID:    "mailgun-api-key",
			input:     `cache_key="key-abcdef1234567890abcdef1234567890"`,
			wantMatch: false,
		},

		// Mailchimp API key
		{
			name:      "mailchimp-api-key matches",
			ruleID:    "mailchimp-api-key",
			input:     "abcdef1234567890abcdef1234567890-us1",
			wantMatch: true,
		},
		{
			name:      "mailchimp-api-key no match without us suffix",
			ruleID:    "mailchimp-api-key",
			input:     "abcdef1234567890abcdef1234567890-eu1",
			wantMatch: false,
		},

		// Databricks PAT
		{
			name:      "databricks-pat matches",
			ruleID:    "databricks-pat",
			input:     "dapi" + strings.Repeat("a", 32),
			wantMatch: true,
		},
		{
			name:      "databricks-pat no match with wrong prefix",
			ruleID:    "databricks-pat",
			input:     "dapi" + strings.Repeat("a", 31),
			wantMatch: false,
		},

		// Grafana service account token
		{
			name:      "grafana-service-account-token matches",
			ruleID:    "grafana-service-account-token",
			input:     "glsa_" + strings.Repeat("a", 32) + "_1a2b3c4d",
			wantMatch: true,
		},
		{
			name:      "grafana-service-account-token no match with non-hex suffix",
			ruleID:    "grafana-service-account-token",
			input:     "glsa_" + strings.Repeat("a", 32) + "_GGGGGGGG",
			wantMatch: false,
		},

		// Grafana Cloud API token
		{
			name:      "grafana-cloud-api-token matches",
			ruleID:    "grafana-cloud-api-token",
			input:     "glc_" + strings.Repeat("a", 32),
			wantMatch: true,
		},
		{
			name:      "grafana-cloud-api-token no match with wrong prefix",
			ruleID:    "grafana-cloud-api-token",
			input:     "glb_" + strings.Repeat("a", 32),
			wantMatch: false,
		},

		// New Relic user API key
		{
			name:      "newrelic-user-api-key matches",
			ruleID:    "newrelic-user-api-key",
			input:     "NRAK-" + strings.Repeat("A", 27),
			wantMatch: true,
		},
		{
			name:      "newrelic-user-api-key no match on short value",
			ruleID:    "newrelic-user-api-key",
			input:     "NRAK-SHORT",
			wantMatch: false,
		},

		// New Relic insert/browser key
		{
			name:      "newrelic-insert-key NRII matches",
			ruleID:    "newrelic-insert-key",
			input:     "NRII-" + strings.Repeat("A", 27),
			wantMatch: true,
		},
		{
			name:      "newrelic-insert-key NRIJ matches",
			ruleID:    "newrelic-insert-key",
			input:     "NRIJ-" + strings.Repeat("B", 27),
			wantMatch: true,
		},
		{
			name:      "newrelic-insert-key NRIS matches",
			ruleID:    "newrelic-insert-key",
			input:     "NRIS-" + strings.Repeat("C", 27),
			wantMatch: true,
		},
		{
			name:      "newrelic-insert-key no match with wrong prefix",
			ruleID:    "newrelic-insert-key",
			input:     "NRAX-" + strings.Repeat("A", 27),
			wantMatch: false,
		},

		// Postman API token
		{
			name:      "postman-api-token matches",
			ruleID:    "postman-api-token",
			input:     "PMAK-abcdef1234567890abcdef12-abcdef1234567890abcdef1234567890abcd12",
			wantMatch: true,
		},
		{
			name:      "postman-api-token no match with wrong prefix",
			ruleID:    "postman-api-token",
			input:     "PMAM-abcdef1234567890abcdef12-abcdef1234567890abcdef1234567890abcd12",
			wantMatch: false,
		},

		// Doppler API token
		{
			name:      "doppler-api-token matches",
			ruleID:    "doppler-api-token",
			input:     "dp.pt." + strings.Repeat("a", 43),
			wantMatch: true,
		},
		{
			name:      "doppler-api-token no match with wrong prefix",
			ruleID:    "doppler-api-token",
			input:     "dp.st." + strings.Repeat("a", 43),
			wantMatch: false,
		},

		// Sentry org token
		{
			name:      "sentry-org-token matches",
			ruleID:    "sentry-org-token",
			input:     "sntrys_eyJ" + strings.Repeat("A", 50),
			wantMatch: true,
		},
		{
			name:      "sentry-org-token no match with wrong prefix",
			ruleID:    "sentry-org-token",
			input:     "sntryx_eyJ" + strings.Repeat("A", 50),
			wantMatch: false,
		},

		// Sentry user token
		{
			name:      "sentry-user-token matches",
			ruleID:    "sentry-user-token",
			input:     "sntryu_" + strings.Repeat("a", 64),
			wantMatch: true,
		},
		{
			name:      "sentry-user-token no match with wrong prefix",
			ruleID:    "sentry-user-token",
			input:     "sntryu_" + strings.Repeat("g", 64),
			wantMatch: false,
		},

		// Atlassian API token
		{
			name:      "atlassian-api-token matches",
			ruleID:    "atlassian-api-token",
			input:     "ATATT3" + strings.Repeat("A", 100),
			wantMatch: true,
		},
		{
			name:      "atlassian-api-token no match on short value",
			ruleID:    "atlassian-api-token",
			input:     "ATATT3short",
			wantMatch: false,
		},

		// Pulumi API token
		{
			name:      "pulumi-api-token matches",
			ruleID:    "pulumi-api-token",
			input:     "pul-" + strings.Repeat("a", 40),
			wantMatch: true,
		},
		{
			name:      "pulumi-api-token no match with wrong prefix",
			ruleID:    "pulumi-api-token",
			input:     "pub-" + strings.Repeat("a", 40),
			wantMatch: false,
		},

		// Fly.io access token
		{
			name:      "flyio-access-token matches",
			ruleID:    "flyio-access-token",
			input:     "fo1_" + strings.Repeat("a", 43),
			wantMatch: true,
		},
		{
			name:      "flyio-access-token no match with wrong prefix",
			ruleID:    "flyio-access-token",
			input:     "fo2_" + strings.Repeat("a", 43),
			wantMatch: false,
		},

		// Mapbox API token
		{
			name:      "mapbox-api-token matches",
			ruleID:    "mapbox-api-token",
			input:     "pk." + strings.Repeat("a", 60) + "." + strings.Repeat("b", 22),
			wantMatch: true,
		},
		{
			name:      "mapbox-api-token no match with wrong prefix",
			ruleID:    "mapbox-api-token",
			input:     "sk." + strings.Repeat("a", 60) + "." + strings.Repeat("b", 22),
			wantMatch: false,
		},

		// Perplexity API key
		{
			name:      "perplexity-api-key matches",
			ruleID:    "perplexity-api-key",
			input:     "pplx-" + strings.Repeat("a", 48),
			wantMatch: true,
		},
		{
			name:      "perplexity-api-key no match on short value",
			ruleID:    "perplexity-api-key",
			input:     "pplx-short",
			wantMatch: false,
		},

		// RubyGems API token
		{
			name:      "rubygems-api-token matches",
			ruleID:    "rubygems-api-token",
			input:     "rubygems_" + strings.Repeat("a", 48),
			wantMatch: true,
		},
		{
			name:      "rubygems-api-token no match on short value",
			ruleID:    "rubygems-api-token",
			input:     "rubygems_short",
			wantMatch: false,
		},

		// Alibaba Cloud access key ID
		{
			name:      "alibaba-access-key-id matches",
			ruleID:    "alibaba-access-key-id",
			input:     "LTAI" + strings.Repeat("A", 20),
			wantMatch: true,
		},
		{
			name:      "alibaba-access-key-id no match on short value",
			ruleID:    "alibaba-access-key-id",
			input:     "LTAI" + strings.Repeat("A", 5),
			wantMatch: false,
		},

		// Authorization Bearer header
		{
			name:      "authorization-bearer matches",
			ruleID:    "authorization-bearer",
			input:     "Authorization: Bearer abcdefghijklmnopqrstu",
			wantMatch: true,
		},
		{
			name:      "authorization-bearer no match on short token",
			ruleID:    "authorization-bearer",
			input:     "Authorization: Bearer short",
			wantMatch: false,
		},

		// Hardcoded password string
		{
			name:      "hardcoded-password-string matches",
			ruleID:    "hardcoded-password-string",
			input:     `password = "mysecretpassword"`,
			wantMatch: true,
		},
		{
			name:      "hardcoded-password-string no match on short value",
			ruleID:    "hardcoded-password-string",
			input:     `password = "short"`,
			wantMatch: false,
		},

		// No match: normal code patterns
		{
			name:      "no match on plain function",
			ruleID:    "aws-access-key-id",
			input:     "func handleRequest(w http.ResponseWriter, r *http.Request) {}",
			wantMatch: false,
		},
	}

	// Build a lookup map from rule ID to Rule.
	ruleMap := make(map[string]Rule, len(DefaultRules))
	for _, r := range DefaultRules {
		ruleMap[r.ID] = r
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule, ok := ruleMap[tc.ruleID]
			if !ok {
				t.Fatalf("rule %q not found in DefaultRules", tc.ruleID)
			}
			got := rule.Pattern.MatchString(tc.input)
			if got != tc.wantMatch {
				if tc.wantMatch {
					t.Errorf("expected rule %q to match %q but it did not", tc.ruleID, tc.input)
				} else {
					t.Errorf("expected rule %q NOT to match %q but it did", tc.ruleID, tc.input)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ScanFile
// ---------------------------------------------------------------------------

func TestScanFile(t *testing.T) {
	t.Run("file with AWS key returns finding", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.env")
		if err := os.WriteFile(path, []byte("AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanFile(path)
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		if len(findings) == 0 {
			t.Fatal("expected at least one finding, got none")
		}
		found := false
		for _, f := range findings {
			if f.Rule.ID == "aws-access-key-id" {
				found = true
				if f.Line != 1 {
					t.Errorf("expected line 1, got %d", f.Line)
				}
				// Match should be redacted (not the raw key)
				if f.Match == "AKIA1234567890ABCDEF" {
					t.Error("match should be redacted but returned raw value")
				}
			}
		}
		if !found {
			t.Error("expected finding with rule ID aws-access-key-id")
		}
	})

	t.Run("clean file returns no findings", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "main.go")
		content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanFile(path)
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		if len(findings) != 0 {
			t.Errorf("expected no findings, got %d: %+v", len(findings), findings)
		}
	})

	t.Run("binary file with null byte returns no findings", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "binary.bin")
		// Embed a null byte among otherwise suspicious-looking data.
		content := []byte("AKIA1234567890ABCDEF\x00binary data here")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanFile(path)
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		if len(findings) != 0 {
			t.Errorf("expected no findings for binary file, got %d", len(findings))
		}
	})

	t.Run("multiline file reports correct line numbers", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "secrets.txt")
		content := "line one\nline two\nAKIA1234567890ABCDEF\nline four\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanFile(path)
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		if len(findings) == 0 {
			t.Fatal("expected findings")
		}
		for _, f := range findings {
			if f.Rule.ID == "aws-access-key-id" && f.Line != 3 {
				t.Errorf("expected AWS key on line 3, got line %d", f.Line)
			}
		}
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		sc := New(DefaultRules)
		_, err := sc.ScanFile("/nonexistent/path/to/file.txt")
		if err == nil {
			t.Error("expected error for nonexistent file, got nil")
		}
	})

	t.Run("JWT finding stores redacted match and context", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "token.txt")
		raw := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123"
		if err := os.WriteFile(path, []byte("token="+raw+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanFile(path)
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		found := false
		for _, f := range findings {
			if f.Rule.ID == "jwt-token" {
				found = true
				if f.Match == raw {
					t.Error("match should be redacted")
				}
				if strings.Contains(f.Context, raw) {
					t.Error("context should not contain raw token")
				}
			}
		}
		if !found {
			t.Error("expected jwt-token finding")
		}
	})
}

// ---------------------------------------------------------------------------
// ScanDir
// ---------------------------------------------------------------------------

// initGitRepo creates a git repo in dir, writes files, and tracks them.
func initGitRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s (%v)", args, out, err)
		}
	}
	run("init")
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "-A")
	run("commit", "-m", "init")
}

func TestScanDir(t *testing.T) {
	t.Run("aggregates findings from multiple files", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir, map[string]string{
			"aws.env":    "AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF\n",
			"stripe.env": "STRIPE_KEY=sk_live_1234567890abcdefghijklmn\n",
			"clean.go":   "package main\nfunc main() {}\n",
		})

		sc := New(DefaultRules)
		findings, err := sc.ScanDir(dir)
		if err != nil {
			t.Fatalf("ScanDir error: %v", err)
		}
		ruleIDs := make(map[string]bool)
		for _, f := range findings {
			ruleIDs[f.Rule.ID] = true
		}
		if !ruleIDs["aws-access-key-id"] {
			t.Error("expected aws-access-key-id finding")
		}
		if !ruleIDs["stripe-live-secret-key"] {
			t.Error("expected stripe-live-secret-key finding")
		}
	})

	t.Run("does not include .git internal files", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir, map[string]string{
			"clean.txt": "nothing here\n",
		})
		// .git/config etc are never in git ls-files output
		sc := New(DefaultRules)
		findings, err := sc.ScanDir(dir)
		if err != nil {
			t.Fatalf("ScanDir error: %v", err)
		}
		for _, f := range findings {
			if strings.Contains(f.File, ".git") {
				t.Errorf("finding should not come from .git directory: %s", f.File)
			}
		}
	})

	t.Run("gitignored files are excluded", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir, map[string]string{
			".gitignore": "secrets/\n",
			"clean.txt":  "nothing\n",
		})
		// Add an untracked file in the ignored dir — git ls-files won't list it
		secretsDir := filepath.Join(dir, "secrets")
		if err := os.MkdirAll(secretsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(secretsDir, "key.txt"), []byte("AKIA1234567890ABCDEF\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanDir(dir)
		if err != nil {
			t.Fatalf("ScanDir error: %v", err)
		}
		for _, f := range findings {
			if strings.Contains(f.File, "secrets") {
				t.Errorf("finding from gitignored dir: %s", f.File)
			}
		}
	})

	t.Run("tracked symlink to a directory is skipped, not fatal", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir, map[string]string{
			"docs/real.txt": "nothing here\n",
			"aws.env":       "AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF\n",
		})
		if err := os.Symlink("docs", filepath.Join(dir, "docs-link")); err != nil {
			t.Fatal(err)
		}
		run := func(args ...string) {
			cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
			cmd.Env = append(
				os.Environ(),
				"GIT_AUTHOR_NAME=test",
				"GIT_AUTHOR_EMAIL=t@t",
				"GIT_COMMITTER_NAME=test",
				"GIT_COMMITTER_EMAIL=t@t",
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %s (%v)", args, out, err)
			}
		}
		run("add", "-A")
		run("commit", "-m", "add symlink")

		var skipped []string
		sc := New(DefaultRules)
		sc.OnSkip = func(sk SkippedFile) {
			skipped = append(skipped, sk.Path)
		}
		findings, err := sc.ScanDir(dir)
		if err != nil {
			t.Fatalf("ScanDir error: %v", err)
		}
		found := false
		for _, f := range findings {
			if f.Rule.ID == "aws-access-key-id" {
				found = true
			}
		}
		if !found {
			t.Error("expected aws-access-key-id finding from regular file")
		}
		linkSkipped := false
		for _, p := range skipped {
			if strings.HasSuffix(p, "docs-link") {
				linkSkipped = true
			}
		}
		if !linkSkipped {
			t.Errorf("expected docs-link to be reported as skipped, got skips: %v", skipped)
		}
	})

	t.Run("binary file in dir returns no findings for that file", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir, map[string]string{
			"text.txt": "clean\n",
		})
		// Add binary after init so it's tracked but binary
		binary := []byte("AKIA1234567890ABCDEF\x00binary")
		if err := os.WriteFile(filepath.Join(dir, "binary.bin"), binary, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "-C", dir, "add", "binary.bin")
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add: %s (%v)", out, err)
		}
		cmd = exec.Command("git", "-C", dir, "commit", "-m", "add binary")
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %s (%v)", out, err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanDir(dir)
		if err != nil {
			t.Fatalf("ScanDir error: %v", err)
		}
		for _, f := range findings {
			if filepath.Base(f.File) == "binary.bin" {
				t.Errorf("should not have findings from binary.bin")
			}
		}
	})

	t.Run("empty repo returns no findings", func(t *testing.T) {
		dir := t.TempDir()
		cmd := exec.Command("git", "-C", dir, "init")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %s (%v)", out, err)
		}

		sc := New(DefaultRules)
		findings, err := sc.ScanDir(dir)
		if err != nil {
			t.Fatalf("ScanDir error: %v", err)
		}
		if len(findings) != 0 {
			t.Errorf("expected no findings in empty repo, got %d", len(findings))
		}
	})

	t.Run("findings include correct file paths", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir, map[string]string{
			"creds.env": "AKIA1234567890ABCDEF\n",
		})

		sc := New(DefaultRules)
		findings, err := sc.ScanDir(dir)
		if err != nil {
			t.Fatalf("ScanDir error: %v", err)
		}
		if len(findings) == 0 {
			t.Fatal("expected findings")
		}
		expected := filepath.Join(dir, "creds.env")
		if findings[0].File != expected {
			t.Errorf("expected file path %q, got %q", expected, findings[0].File)
		}
	})
}

// ---------------------------------------------------------------------------
// IgnoreConfig.ShouldIgnore
// ---------------------------------------------------------------------------

func TestIgnoreConfig_ShouldIgnore(t *testing.T) {
	awsRule := Rule{
		ID:       "aws-access-key-id",
		Severity: "high",
	}

	makeFinding := func(ruleID, file, match, ctx string) Finding {
		return Finding{
			File:     file,
			Line:     1,
			Column:   1,
			Rule:     Rule{ID: ruleID, Severity: "high"},
			RawMatch: "",
			Match:    match,
			Context:  ctx,
		}
	}
	_ = awsRule

	t.Run("suppresses by rule ID exact match", func(t *testing.T) {
		ic := &IgnoreConfig{
			Rules: []string{"aws-access-key-id"},
		}
		f := makeFinding(
			"aws-access-key-id",
			"/some/file.go",
			"AKIA****",
			"export AWS_KEY=AKIA****",
		)
		if !ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=true for matching rule ID")
		}
	})

	t.Run("suppresses by rule ID case-insensitive", func(t *testing.T) {
		ic := &IgnoreConfig{
			Rules: []string{"AWS-ACCESS-KEY-ID"},
		}
		f := makeFinding("aws-access-key-id", "/some/file.go", "AKIA****", "")
		if !ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=true for case-insensitive rule ID match")
		}
	})

	t.Run("does not suppress unrelated rule ID", func(t *testing.T) {
		ic := &IgnoreConfig{
			Rules: []string{"stripe-live-secret-key"},
		}
		f := makeFinding("aws-access-key-id", "/some/file.go", "AKIA****", "")
		if ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=false for non-matching rule ID")
		}
	})

	t.Run("suppresses by exact path glob", func(t *testing.T) {
		ic := &IgnoreConfig{
			Paths: []string{"/fixtures/testdata.env"},
		}
		f := makeFinding("aws-access-key-id", "/fixtures/testdata.env", "AKIA****", "")
		if !ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=true for matching path")
		}
	})

	t.Run("suppresses by wildcard path glob", func(t *testing.T) {
		ic := &IgnoreConfig{
			Paths: []string{"**/testdata/**"},
		}
		f := makeFinding(
			"aws-access-key-id",
			"/project/testdata/fixtures/creds.env",
			"AKIA****",
			"",
		)
		if !ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=true for glob path match")
		}
	})

	t.Run("does not suppress unmatched path", func(t *testing.T) {
		ic := &IgnoreConfig{
			Paths: []string{"/fixtures/testdata.env"},
		}
		f := makeFinding("aws-access-key-id", "/production/real.env", "AKIA****", "")
		if ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=false for non-matching path")
		}
	})

	t.Run("suppresses by pattern match in match field", func(t *testing.T) {
		ic := &IgnoreConfig{
			Patterns: []string{"EXAMPLE"},
		}
		f := makeFinding(
			"aws-access-key-id",
			"/some/file.go",
			"AKIA**EXAMPLE",
			"export KEY=AKIA**EXAMPLE",
		)
		if !ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=true for pattern match in match field")
		}
	})

	t.Run("suppresses by pattern match in context field", func(t *testing.T) {
		ic := &IgnoreConfig{
			Patterns: []string{"EXAMPLE"},
		}
		f := makeFinding(
			"aws-access-key-id",
			"/some/file.go",
			"AKIA****",
			"export KEY=AKIAEXAMPLE1234",
		)
		if !ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=true for pattern match in context field")
		}
	})

	t.Run("does not suppress when pattern does not match", func(t *testing.T) {
		ic := &IgnoreConfig{
			Patterns: []string{"EXAMPLE"},
		}
		f := makeFinding(
			"aws-access-key-id",
			"/some/file.go",
			"AKIA****",
			"export KEY=AKIA1234567890ABCD",
		)
		if ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=false when pattern does not match")
		}
	})

	t.Run("empty config never ignores", func(t *testing.T) {
		ic := &IgnoreConfig{}
		f := makeFinding("aws-access-key-id", "/some/file.go", "AKIA****", "")
		if ic.ShouldIgnore(f) {
			t.Error("expected ShouldIgnore=false for empty config")
		}
	})

	t.Run("double-star glob matches relative paths from history scan", func(t *testing.T) {
		ic := &IgnoreConfig{
			Paths: []string{
				"**/tools/suspenders/**/*_test.go",
				"**/tools/suspenders/**/rules.go",
				"**/tools/suspenders/README.md",
			},
		}
		for _, tt := range []struct {
			file string
			want bool
		}{
			{"tools/suspenders/internal/scanner/scanner_test.go", true},
			{"tools/suspenders/internal/scanner/rules.go", true},
			{"tools/suspenders/README.md", true},
			{"tools/suspenders/internal/scanner/scanner.go", false},
			{"/abs/tools/suspenders/internal/scanner/scanner_test.go", true},
		} {
			f := makeFinding("aws-access-key-id", tt.file, "AKIA****", "")
			got := ic.ShouldIgnore(f)
			if got != tt.want {
				t.Errorf("ShouldIgnore(file=%q) = %v, want %v", tt.file, got, tt.want)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// LoadIgnoreConfig
// ---------------------------------------------------------------------------

func TestLoadIgnoreConfig(t *testing.T) {
	t.Run("loads valid yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".suspenders.yaml")
		content := `rules:
  - aws-access-key-id
  - stripe-live-secret-key
paths:
  - "**/testdata/**"
patterns:
  - EXAMPLE
`
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		ic, err := LoadIgnoreConfig(path)
		if err != nil {
			t.Fatalf("LoadIgnoreConfig error: %v", err)
		}
		if len(ic.Rules) != 2 {
			t.Errorf("expected 2 rules, got %d", len(ic.Rules))
		}
		if len(ic.Paths) != 1 {
			t.Errorf("expected 1 path, got %d", len(ic.Paths))
		}
		if len(ic.Patterns) != 1 {
			t.Errorf("expected 1 pattern, got %d", len(ic.Patterns))
		}
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		_, err := LoadIgnoreConfig("/nonexistent/.suspenders.yaml")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("returns error for invalid yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".suspenders.yaml")
		if err := os.WriteFile(path, []byte("rules: [\ninvalid"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadIgnoreConfig(path)
		if err == nil {
			t.Error("expected error for invalid yaml, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Integration: ScanFile respects IgnoreFile
// ---------------------------------------------------------------------------

func TestScanFile_WithIgnoreFile(t *testing.T) {
	t.Run("ignored rule ID suppresses finding", func(t *testing.T) {
		dir := t.TempDir()

		ignorePath := filepath.Join(dir, ".suspenders.yaml")
		ignoreContent := "rules:\n  - aws-access-key-id\n"
		if err := os.WriteFile(ignorePath, []byte(ignoreContent), 0o600); err != nil {
			t.Fatal(err)
		}

		scanPath := filepath.Join(dir, "creds.env")
		if err := os.WriteFile(scanPath, []byte("AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		sc.IgnoreFile = ignorePath

		findings, err := sc.ScanFile(scanPath)
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		for _, f := range findings {
			if f.Rule.ID == "aws-access-key-id" {
				t.Error("aws-access-key-id finding should have been suppressed by ignore config")
			}
		}
	})

	t.Run("non-matching ignore rule does not suppress finding", func(t *testing.T) {
		dir := t.TempDir()

		ignorePath := filepath.Join(dir, ".suspenders.yaml")
		ignoreContent := "rules:\n  - stripe-live-secret-key\n"
		if err := os.WriteFile(ignorePath, []byte(ignoreContent), 0o600); err != nil {
			t.Fatal(err)
		}

		scanPath := filepath.Join(dir, "creds.env")
		if err := os.WriteFile(scanPath, []byte("AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		sc := New(DefaultRules)
		sc.IgnoreFile = ignorePath

		findings, err := sc.ScanFile(scanPath)
		if err != nil {
			t.Fatalf("ScanFile error: %v", err)
		}
		found := false
		for _, f := range findings {
			if f.Rule.ID == "aws-access-key-id" {
				found = true
			}
		}
		if !found {
			t.Error("expected aws-access-key-id finding to be present when not in ignore list")
		}
	})
}

// TestScanDirNonGit pins the fallback path: scanning a directory that is not
// a git working tree walks the filesystem instead of failing on git plumbing.
func TestScanDirNonGit(t *testing.T) {
	t.Run("finds secrets in a plain directory", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"aws.env":         "AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF\n",
			"nested/deep.env": "STRIPE_KEY=sk_live_1234567890abcdefghijklmn\n",
			"clean.go":        "package main\nfunc main() {}\n",
		})

		sc := New(DefaultRules)
		findings, err := sc.ScanDir(dir)
		if err != nil {
			t.Fatalf("ScanDir error: %v", err)
		}
		ruleIDs := make(map[string]bool)
		for _, f := range findings {
			ruleIDs[f.Rule.ID] = true
		}
		if !ruleIDs["aws-access-key-id"] {
			t.Error("expected aws-access-key-id finding")
		}
		if !ruleIDs["stripe-live-secret-key"] {
			t.Error("expected stripe-live-secret-key finding")
		}
	})

	t.Run("skips .git directories of nested repositories", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"clean.txt":                   "nothing here\n",
			"vendored/.git/leaked.txt":    "AKIA1234567890ABCDEF\n",
			"vendored/tracked-secret.env": "AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF\n",
		})

		sc := New(DefaultRules)
		findings, err := sc.ScanDir(dir)
		if err != nil {
			t.Fatalf("ScanDir error: %v", err)
		}
		var sawNested bool
		for _, f := range findings {
			rel, err := filepath.Rel(dir, f.File)
			if err != nil {
				t.Fatalf("Rel(%q, %q): %v", dir, f.File, err)
			}
			if strings.Contains(rel, ".git") {
				t.Errorf("finding should not come from a .git directory: %s", rel)
			}
			if strings.Contains(rel, "tracked-secret.env") {
				sawNested = true
			}
		}
		if !sawNested {
			t.Error("expected finding from the nested repo's working tree")
		}
	})
}

// writeFiles writes rel -> content files under dir, creating parents.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
