#!/bin/sh
# Launch myservice with its secret resolved at RUNTIME — no value ever lands in
# a tracked file. Two resolution styles are shown; keep the one that fits.
set -eu

# Pattern 1 — resolve from an external store (preferred). Swap the resolver for
# your own and export what the service reads. The value lives only in this
# process. Examples:
#   token="$(security find-generic-password -s myservice -w)"   # macOS keychain
#   token="$(gh auth token)"                                     # gh keychain
#   token="$(pass show myservice/api-token)"                     # pass
#   token="$(op read op://vault/myservice/token)"               # 1Password
# export MYSERVICE_API_TOKEN="$token"

# Pattern 2 — source an uncommitted env file (gitignored; see .gitignore).
env_file="$HOME/.config/myservice/secrets.env"
if [ -f "$env_file" ]; then
  set -a
  . "$env_file"
  set +a
fi

exec "$HOME/code/bin/myservice" serve
