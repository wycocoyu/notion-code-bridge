# Notion Code Bridge development

This is the user's personal development fork of SleepingBag945/notion_manager.
Read README.md and AGENT_THREAD.md before changing the integration.

- `origin` is wycocoyu/notion-code-bridge. `upstream` is the original project.
- Preserve the original project's license and attribution.
- Work on named `codex/` branches and deliver through pull requests. Never push
  master/main directly. Send upstream changes only when the user explicitly asks.
- Check the branch, HEAD and dirty state before editing; preserve unrelated work.
- Personal Agent transport is opt-in and supports Auto plus Opus 5.5. Each inference creates a new
  Notion thread carrying client history; it does not resume the same web chat.
- Local skills are indexed and read from original sources. Read-based loading is
  verified; native Skill/slash-command support is unverified.
- Never commit config.yaml, account cookies, credentials, runtime logs, private
  captures or compiled binaries. Do not display authentication values.
- Run focused protocol tests with `go test ./internal/proxy -run TestAgent -count=1`;
  run `go test ./...` and build before review readiness for runtime changes.
- Distinguish interface/HTTP success from real tool execution. Live tests consume
  account allowance; reuse passing evidence for unchanged runtime code.
- A separate local installation may still run the older checkout. Verify the
  actual executable and working directory before rebuilding/restarting a service.
