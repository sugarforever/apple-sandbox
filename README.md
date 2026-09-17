# apple-sandbox

A no-daemon CLI that runs OpenAI Agents API [self-hosted executors](https://developers.openai.com/api/docs/guides/agents-api/environments/self-hosted)
(`codex exec-server`) inside isolated VMs, using [Apple `container`](https://github.com/apple/container)
as the runtime. Each sandbox session is one OCI container = one lightweight
Virtualization.framework VM.

## Status

MVP. Verified so far:

- `run` → `ls` → `logs` → `stop` → `rm` lifecycle works end-to-end against
  Apple `container` on this machine.
- The executor image builds (Node 22 + `@openai/codex@latest`) and its
  entrypoint runs `codex exec-server`, which itself enforces that `--remote`
  points at an `openai.com`/`openai.org` host — confirmed by testing against
  a bogus URL and seeing that exact rejection surface through `logs`.

Not yet verified: a real end-to-end run against a live Agents API session
(needs real `environment-id` / `remote-url` / restricted `CODEX_API_KEY`
from your own app's session-creation call).

### Known gaps (not in this MVP)

- **No egress allowlisting.** Apple `container` networks only support
  `--internal` (host-only) or open egress — there's no domain allowlist
  primitive. This sandbox currently gives the executor full outbound network
  access rather than restricting it to `api.openai.com` /
  `codex-cloud-environments.chatgpt.com`. Don't use this for untrusted
  workloads until a proxy/allowlist layer is added.
- **No daemon / SDK.** This is CLI-only for now; a TS/Python SDK would just
  wrap these same subprocess calls.
- **No webhook-driven lifecycle.** OpenAI drives self-hosted sessions with
  events (`agent.session.created`, `action_required`, `in_progress`, `idle`,
  `failed`) that production infra is expected to react to automatically. This
  CLI is a manual substitute — `run`/`stop`/`rm` by hand — not a reconciler.
  See [Integrating with the OpenAI Agents API](#integrating-with-the-openai-agents-api).
- **`CODEX_API_KEY` as the container env var is now corroborated** by
  Cloudflare's own reference integration (independent of the earlier
  research this project started from), though still not tested end-to-end
  against a live session on Apple `container` specifically. See the comment
  in [`image/executor/entrypoint.sh`](image/executor/entrypoint.sh).

## Prerequisites

- Apple Silicon Mac, macOS with Apple `container` installed
  (`brew install --cask container`) and its services started
  (`container system start`).

Run `apple-sandbox doctor` to check all of the above plus free disk space.

## Install

Download the latest `darwin_arm64` release from
[Releases](https://github.com/sugarforever/apple-sandbox/releases) (there's
nothing else to build for — Apple `container` doesn't run on any other
platform):

```bash
curl -sL "https://api.github.com/repos/sugarforever/apple-sandbox/releases/latest" \
  | grep -o '"browser_download_url": *"[^"]*darwin_arm64\.tar\.gz"' \
  | cut -d '"' -f4 \
  | xargs curl -sL \
  | tar xz -C /usr/local/bin apple-sandbox
```

Or build from source (needs Go 1.27+):

```bash
go build -o bin/apple-sandbox ./cmd/apple-sandbox
```

## Integrating with the OpenAI Agents API

`apple-sandbox` starts the VM and the executor inside it; everything about
sessions, agents, and events is between your app and OpenAI directly. Full
protocol: OpenAI's [self-hosted sandboxes guide](https://developers.openai.com/api/docs/guides/agents-api/environments/self-hosted).
The clearest worked example of the whole flow today is Cloudflare's own
[OpenAI Agents API tutorial](https://developers.cloudflare.com/sandbox/tutorials/openai-agents-api/) —
it targets Cloudflare Containers rather than Apple `container`, but the
Agents API side (session creation, webhook events, key scopes) is identical.

### 1. Create an agent (once, not per session)

```bash
curl https://api.openai.com/v1/agents \
  --request POST \
  --header "OpenAI-Beta: agents=v1" \
  --header "Authorization: Bearer $OPENAI_API_KEY" \
  --json '{"name": "apple-sandbox-demo", "model": "<model>"}'
```

### 2. Create a self-hosted session

```bash
curl https://api.openai.com/v1/agents/sessions \
  --request POST \
  --header "OpenAI-Beta: agents=v1" \
  --header "Authorization: Bearer $OPENAI_API_KEY" \
  --json '{
    "agent_id": "'"$AGENT_ID"'",
    "environment": {"type": "self_hosted", "workspace_directory": "/workspace"}
  }'
```

The response carries the session id (`sess_...`) plus the environment's `id`
and `remote_url` — these are exactly the `--environment-id` and
`--remote-url` values `apple-sandbox run` expects below.

### 3. Issue a restricted executor key

From the OpenAI Platform, create a key scoped to exactly:

- `api.model.read`
- `api.agents.environments.connect`

Nothing else. This becomes `--executor-key` — the container's `CODEX_API_KEY`
— and must never be your app's own `OPENAI_API_KEY`.

### 4. Start the sandbox, then drive the session normally

```bash
apple-sandbox run \
  --environment-id <environment.id from step 2> \
  --remote-url <environment.remote_url from step 2> \
  --executor-key <restricted key from step 3>
```

Session input goes through the Agents API itself, not through this CLI:

```bash
curl https://api.openai.com/v1/agents/sessions/$SESSION_ID/events \
  --request POST \
  --header "OpenAI-Beta: agents=v1" \
  --header "Authorization: Bearer $OPENAI_API_KEY" \
  --json '{"events": [{"type": "session.input.message",
    "input": [{"role": "user", "content": [{"type": "input_text", "text": "..."}]}]}]}'
```

### What this CLI does not yet automate

OpenAI drives self-hosted sessions with webhook events that production
infrastructure is expected to react to:

| Event | Expected reaction |
|---|---|
| `agent.session.created` | Prewarm the executor VM |
| `agent.session.action_required` | Confirm ownership, read `environment.id`/`remote_url`, start or reconnect the executor |
| `agent.session.in_progress` | Extend the session's lifecycle deadline |
| `agent.session.idle` | Snapshot the VM if supported, then arm a shutdown deadline |
| `agent.session.failed` | Stop the container and clear any snapshot |

`apple-sandbox run` / `stop` / `rm` is a manual, imperative substitute for
that whole reconciliation loop — fine for developing and debugging one
session at a time, but a real deployment needs a webhook receiver that maps
these events onto `container` lifecycle calls automatically. Cloudflare's
reference Worker (one Durable Object per session) is the clearest existing
example of that state machine; porting the same reconciliation logic onto
Apple `container` instead of the manual CLI is the natural next step past
this MVP.

One more sharp edge from the same source: **deleting an OpenAI session does
not itself trigger container cleanup** — your infra has to notice via the
`failed` webhook or a `404` on session lookup and tear the container down
itself. Today, `apple-sandbox rm` only runs when you run it.

## Usage

```bash
# 1. Check the environment
./bin/apple-sandbox doctor

# 2. Build the executor image (Node + codex)
./bin/apple-sandbox build

# 3. Start a session (environment-id / remote-url / executor-key come from
#    the Integrating section above)
./bin/apple-sandbox run \
  --environment-id env_xxx \
  --remote-url https://<agents-api-remote-url> \
  --executor-key <restricted-CODEX_API_KEY>

# 4. Watch it
./bin/apple-sandbox logs -f <session-id>

# 5. List / stop / remove
./bin/apple-sandbox ls
./bin/apple-sandbox stop <session-id>
./bin/apple-sandbox rm <session-id>
```

Each session's `/workspace` is a bind mount of
`./sandboxes/<session-id>/` on the host — that host directory is the durable
state; the container itself is disposable (`rm` after `stop`).

## How it works

`apple-sandbox` only ever plays one role in this flow: turning an
`environment.id` / `remote_url` / restricted key — which your app obtains
from the Agents API itself — into a running `codex exec-server` inside an
Apple `container` VM. It never talks to OpenAI directly.

```mermaid
sequenceDiagram
    actor App as Your app<br/>(OPENAI_API_KEY)
    participant Agents as OpenAI Agents API
    participant CLI as apple-sandbox (local CLI)
    participant VM as Apple container VM<br/>(codex exec-server)
    participant Harness as OpenAI hosted harness

    App->>Agents: 1. Create session (environment.type = self_hosted)
    Agents-->>App: 2. session.environment.id + environment.remote_url
    App->>App: 3. Issue a restricted, environment-scoped CODEX_API_KEY<br/>(scopes: api.model.read + api.agents.environments.connect, nothing else)
    App->>CLI: 4. apple-sandbox run --environment-id --remote-url --executor-key
    CLI->>VM: 5. container run -d (bind-mount ./sandboxes/<id> → /workspace, inject env vars)
    VM->>VM: 6. entrypoint.sh → codex exec-server --remote <url> --environment-id <id>
    VM->>Harness: 7. Outbound WebSocket registration, authenticated with CODEX_API_KEY
    Harness-->>Agents: 8. Session marked active
    Agents-->>App: 9. SSE session events (in_progress / action_required / idle)

    loop for each turn while the session is open
        Harness->>VM: 10. Dispatch a command
        VM->>VM: 11. Execute it against /workspace
        VM-->>Harness: 12. Return result (auto-reconnects the WebSocket if it drops)
    end

    Harness-->>Agents: 13. Relay outputs / artifacts
    App->>Agents: 14. Poll or stream the session, download artifacts
    App->>CLI: 15. apple-sandbox logs / stop / rm to inspect or tear down the VM
```

Two things worth noting from that diagram:

- **Steps 1–3 and 9–14 happen entirely between your app and OpenAI** —
  `apple-sandbox` is invisible to them. It only exists to do step 5 (start
  the VM) and step 6 (launch the executor inside it) correctly, then get out
  of the way while 7–12 run over the executor's own outbound WebSocket.
- **Step 7 is the only network requirement**: the VM needs outbound access
  to `https://api.openai.com` and `wss://codex-cloud-environments.chatgpt.com`.
  As noted in [Known gaps](#known-gaps-not-in-this-mvp), this sandbox doesn't
  yet restrict egress to just those two hosts.

### Local CLI mechanics

What `apple-sandbox run` actually shells out to, for steps 4–6 above:

```
apple-sandbox run
  → creates ./sandboxes/<session-id>/ (host dir, bind-mounted to /workspace)
  → container run -d --name apple-sandbox-<session-id> \
      -v <workspace>:/workspace \
      -e CODEX_API_KEY=... -e OPENAI_ENVIRONMENT_ID=... -e OPENAI_REMOTE_URL=... \
      apple-sandbox-executor:latest
       → entrypoint.sh → codex exec-server --remote ... --environment-id ...
            → outbound WebSocket to OpenAI's hosted harness
```

No daemon, no control-plane process: `apple-sandbox` shells out to the
`container` CLI directly, and the container survives (not `--rm`) after
stopping so `logs` remains available for debugging a crashed executor.

## Releasing

Releases are built by [GoReleaser](https://goreleaser.com/) via
[`.github/workflows/release.yml`](.github/workflows/release.yml), triggered
by pushing a semver tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

That produces a GitHub Release with the `darwin_arm64` archive, a
`checksums.txt`, and an auto-generated changelog (grouped by
`feat:`/`fix:` commit prefixes). [`.github/workflows/ci.yml`](.github/workflows/ci.yml)
runs `gofmt`, `go vet`, `go build`, and a GoReleaser snapshot build (no
publish) on every push/PR to `main`, so a broken release config fails CI
before it ever fails a real tag push.

To dry-run a release locally: `goreleaser release --snapshot --clean --skip=publish`.
