#!/bin/sh
# Registers this container as an OpenAI Agents API self-hosted executor.
#
# Required env (injected by `apple-sandbox run`):
#   OPENAI_EXECUTOR_API_KEY  restricted, environment-scoped key (never the app's main OPENAI_API_KEY)
#   OPENAI_ENVIRONMENT_ID    session.environment.id from the Agents API
#   OPENAI_REMOTE_URL        session.environment.remote_url from the Agents API
#
# codex exec-server itself only reads CODEX_API_KEY — that name isn't ours to
# change. OPENAI_EXECUTOR_API_KEY (matching Cloudflare's reference
# integration's Worker-secret name) is the one name apple-sandbox exposes
# anywhere outside this file; the alias to CODEX_API_KEY happens here and
# nowhere else.
#
# NOTE: verified end-to-end against a real Agents API session — with a
# properly-scoped key, this registered, received a real task, and wrote a
# real output file back to /workspace. See the README's Status section.
set -eu

: "${OPENAI_EXECUTOR_API_KEY:?OPENAI_EXECUTOR_API_KEY is required}"
: "${OPENAI_ENVIRONMENT_ID:?OPENAI_ENVIRONMENT_ID is required}"
: "${OPENAI_REMOTE_URL:?OPENAI_REMOTE_URL is required}"

export CODEX_API_KEY="$OPENAI_EXECUTOR_API_KEY"

exec codex exec-server \
  --remote "$OPENAI_REMOTE_URL" \
  --environment-id "$OPENAI_ENVIRONMENT_ID" \
  "$@"
