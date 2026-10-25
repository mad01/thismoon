#!/bin/sh
# Launch `speak serve` for the speak-web t-man agent, inside its sandbox
# (anthropics/sandbox-runtime, launched through srt-run).
#
# The provider's key comes from the secrets file, read here, outside the
# sandbox, by speak-provider-env.sh: only the variables the active provider
# reads cross into the sandbox, and the secrets file stays unreadable inside
# it. The provider's host differs per machine (the config picks local,
# OpenRouter, OpenAI, a LiteLLM proxy or Gemini, and a block may set its own
# base URL), so `speak config host` names it and srt-run gets it as
# SRT_RUN_ALLOW_DOMAINS, the only egress beyond localhost. The local engine
# prints no host, so speak-web then reaches nothing beyond loopback.
#
# The settings are ~/.config/srt/speak-web.json, which the speak recipe links
# from speak-web.srt.json. The label (speak-web) is the t-man name; the MCP
# server runs unsandboxed through speak-env.sh. Every argument passes through
# to `speak serve` unchanged.
set -eu

# shellcheck source=speak-provider-env.sh
. "$(dirname "$0")/speak-provider-env.sh"

host=$("$SPEAK_BIN" config host 2> /dev/null || true)
if [ -n "$host" ]; then
  SRT_RUN_ALLOW_DOMAINS="${SRT_RUN_ALLOW_DOMAINS:+$SRT_RUN_ALLOW_DOMAINS,}$host"
  export SRT_RUN_ALLOW_DOMAINS
fi

cd /
exec "$HOME/code/bin/srt-run" "$HOME/.config/srt/speak-web.json" \
  "$SPEAK_BIN" serve "$@"
