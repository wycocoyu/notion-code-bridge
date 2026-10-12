#!/usr/bin/env python3
"""Run an isolated Claude Code process against the local Notion Auto proxy."""
import os
import shutil
import subprocess
import sys
import time
import urllib.request
from pathlib import Path
import yaml

ROOT = Path(__file__).resolve().parent
cfg = yaml.safe_load((ROOT / "config.yaml").read_text())
port = cfg["server"].get("port", "8081")
base = "http://127.0.0.1:" + str(port)
key = cfg["server"]["api_key"]
if not key:
    raise SystemExit("Initialize notion-manager first so config.yaml contains a local API key")
if not cfg["proxy"].get("agent_thread"):
    raise SystemExit("Enable proxy.agent_thread in config.yaml first")

def ready():
    req = urllib.request.Request(base + "/v1/models", headers={"Authorization": "Bearer " + key})
    try:
        with urllib.request.urlopen(req, timeout=1) as response:
            return response.status == 200
    except Exception:
        return False

if not ready():
    log = ROOT / "_claude-notion.log"
    with log.open("ab") as output:
        log.chmod(0o600)
        proxy_env = os.environ.copy()
        for name in ("PORT", "API_KEY", "NOTION_API_BASE", "DEFAULT_MODEL", "AGENT_THREAD"):
            proxy_env.pop(name, None)
        proxy_env["HOST"] = "127.0.0.1"
        process = subprocess.Popen([str(ROOT / "notion-manager")], cwd=ROOT, env=proxy_env,
                                   stdout=output, stderr=output, start_new_session=True)
    deadline = time.monotonic() + 15
    while not ready():
        if process.poll() is not None or time.monotonic() >= deadline:
            raise SystemExit("Notion proxy failed to start; see " + str(log))
        time.sleep(0.3)
    (ROOT / "_claude-notion.pid").write_text(str(process.pid))

env = os.environ.copy()
for name in list(env):
    if name.startswith("ANTHROPIC_"):
        env.pop(name)
env.update(ANTHROPIC_BASE_URL=base, ANTHROPIC_API_KEY=key,
           CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1",
           NO_PROXY="127.0.0.1,localhost")

# Advertise installed skills without copying their bodies or changing sources.
# Project skills take precedence; shared installs are deduplicated by name.
skills = {}
for directory in (Path.cwd() / ".claude/skills", Path.home() / ".claude/skills",
                  Path.home() / ".agents/skills"):
    if not directory.is_dir():
        continue
    for entry in sorted(directory.iterdir()):
        source = entry / "SKILL.md"
        if not source.is_file():
            continue
        try:
            text = source.read_text()
            if not text.startswith("---"):
                continue
            metadata = yaml.safe_load(text.split("---", 2)[1]) or {}
            if not isinstance(metadata, dict):
                continue
            name = str(metadata.get("name") or entry.name)
            skills.setdefault(name, (str(metadata.get("description") or ""), str(source.resolve())))
        except (OSError, ValueError, yaml.YAMLError):
            continue
catalog = "\n".join(f"- {name}: {description}\n  Source: {source}"
                    for name, (description, source) in skills.items())
skill_context = (
    "Local skills are available below. Select a matching skill by its description. "
    "Use the Skill tool to load an installed Claude skill; if it cannot resolve the name, "
    "use Read on the exact SKILL.md source. Read the skill before following it. "
    "Do not modify the skill source unless the user requests it.\n" + catalog
)
claude = shutil.which("claude")
if not claude:
    raise SystemExit("Claude Code is not installed or is not on PATH")
args = [claude, "--bare", "--setting-sources", "",
        "--strict-mcp-config", "--model", "auto",
        "--tools", "Read,Edit,Write,Bash,Glob,Grep,Skill",
        "--system-prompt", "You are a coding assistant helping the user work in their current directory. Use local tools to read before editing and to run relevant tests. Report observed results honestly.",
        "--append-system-prompt", skill_context]
os.execve(args[0], args + sys.argv[1:], env)
