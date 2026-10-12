# Current Notion Agent transport (experimental)

The current Notion personal Agent UI can use `createAgentThread` and
`getThreadTranscript` instead of the older `runInferenceTranscript` workflow.
This opt-in transport adapts those responses to the existing Anthropic/OpenAI
API and local tool JSON bridge. The legacy transport remains the default.

## Configuration

```yaml
proxy:
  agent_thread: true
  notion_api_base: https://app.notion.com/api/v3
  default_model: auto
```

Environment overrides: `AGENT_THREAD=true`,
`NOTION_API_BASE=https://app.notion.com/api/v3`, and `DEFAULT_MODEL=auto`.
Use the existing account files and local API-key configuration. Never publish
account cookies or keys.

Auto omits the upstream model field. Each inference creates a fresh thread
and carries the complete conversation supplied by the client, including local
function calls and tool results. It does not resume a Notion web conversation.
Roles are rendered as text, not preserved as native model API roles. Local
functions are requested as JSON and executed by the client, not Notion.

The transport disables Notion browser access and suggested edits, excludes
chats from Notion memories, and requests approval mode. It polls until both an
`end_turn` completion and an idle session are observed, bounded by the configured
inference timeout. A request timeout does not cancel the remote Notion turn;
client retries create another thread and may produce different output.

## Claude Code

Point `ANTHROPIC_BASE_URL` to the running proxy, supply its local key through
`ANTHROPIC_API_KEY`, and select `--model auto`. Scope these variables to the
process rather than changing unrelated provider settings.

An optional portable launcher is included:

```bash
go build -o notion-manager ./cmd/notion-manager
python3 -m venv .venv
. .venv/bin/activate
python -m pip install PyYAML
# Configure config.yaml and import your own Notion account first.
python claude-notion.py
```

Run the launcher from the project directory you want to edit, using an absolute
path to `claude-notion.py` when it is elsewhere. It finds Claude Code on PATH,
loads the local configuration key without printing it, and starts the proxy on
loopback when needed. It does not overwrite global Claude settings.

The launcher uses bare mode and a concise coding prompt. It advertises local
skills from project `.claude/skills`, user `.claude/skills`, and user
`.agents/skills`, in that order. Bodies are read on demand from the original
files. Read-based skill loading was verified; native slash-command/Skill-tool
support is not claimed. Plugins, hooks, external MCP and automatic CLAUDE.md
discovery are not enabled; normal interactive permissions remain active.

## Validation and limits

Focused tests cover observed transcript shapes, updates/removals, ordering,
incomplete pages, failure states, total deadlines, Auto tool schemas and complete
conversation/tool-result rendering:

```bash
go test ./internal/proxy -run TestAgent -count=1
go test ./internal/proxy -count=1
```

Local live smoke tests exercised file reading, Edit, Bash test execution and
Read-based local skill loading with a user-owned account. These do not establish
long-context compression, concurrent/resumed-session reliability, or which
vendor model Auto selected. No credentials or private captures are included.

Auto and Opus 5.5 are supported. Attachments and transcript pages requiring pagination
are rejected. Output is collected before Anthropic SSE delivery, so upstream
token-by-token streaming is not implemented. Code/conversation context is sent
to Notion. This bridge is experimental, not a complete native Claude API.

## Explicit Opus 5.5 selection

```bash
python claude-notion.py --model opus-5.5
```

Supported aliases are `opus-5.5`, `claude-opus-5-5`, `claude-opus-5.5` and the
observed Notion ID `albuquerque-quinn`. The Agent request sends that exact ID
and `reasoningEffort: medium`, matching the observed browser selection.
Unknown models are rejected; this route does not use the legacy family fallback.
Auto continues to omit the model field. Availability and allowance remain
controlled by the selected Notion account/workspace; listing a model alone does
not prove inference access. The user's upgraded account was verified with a
selected Opus 5.5 browser request and a completed answer.
