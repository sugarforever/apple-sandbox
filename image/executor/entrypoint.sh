#!/bin/sh
# Registers this container as an OpenAI Agents API self-hosted executor.
#
# Required env (injected by `apple-sandbox run`):
#   CODEX_API_KEY          restricted, environment-scoped key (never the app's main OPENAI_API_KEY)
#   OPENAI_ENVIRONMENT_ID  session.environment.id from the Agents API
#   OPENAI_REMOTE_URL      session.environment.remote_url from the Agents API
#
# NOTE: verified end-to-end against a real Agents API session — with a
# properly-scoped CODEX_API_KEY, this registered, received a real task, and
# wrote a real output file back to /workspace. See the README's Status
# section.
set -eu

: "${CODEX_API_KEY:?CODEX_API_KEY is required}"
: "${OPENAI_ENVIRONMENT_ID:?OPENAI_ENVIRONMENT_ID is required}"
: "${OPENAI_REMOTE_URL:?OPENAI_REMOTE_URL is required}"

exec codex exec-server \
  --remote "$OPENAI_REMOTE_URL" \
  --environment-id "$OPENAI_ENVIRONMENT_ID" \
  "$@"
