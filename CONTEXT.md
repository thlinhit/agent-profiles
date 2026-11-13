# agent-profiles

Switches config profiles for AI coding agents (accounts, settings, MCP servers, skills) per shell and per directory.

## Language

### Agents and profiles

**Agent**:
A supported AI coding tool that reads its config from a directory named by an environment variable.
_Avoid_: tool, client, provider

**Adapter**:
The code that teaches `ap` one Agent's env var, default config dir, init template and optional capabilities.
_Avoid_: plugin, driver

**Profile**:
A named, private config directory for one Agent, kept in profile storage. Its path is the value of the Agent's env var.
_Avoid_: account, default profile

**Profile contents**:
Everything an Agent stores in its config dir: login, settings, MCP servers, skills, sessions and history.
_Avoid_: profile data, profile state

**Default config dir**:
The Agent's own built-in config directory (for example `~/.claude`). It is not a Profile.
_Avoid_: default profile

**Profile storage**:
The private directory tree `$XDG_DATA_HOME/agent-profiles/<agent>/<name>` that holds every Profile.
_Avoid_: profile database, registry

### Selection

**Selection**:
A pairing of one Agent with one Profile, coming from a trusted Dotfile.
_Avoid_: binding, mapping

**Activate**:
Export an Agent's env var in the current shell so it reads a Profile (`use`). The reverse is **deactivate** (`off`).
_Avoid_: switch (as a noun), apply

**Active profile**:
The config dir an Agent will actually read right now: a Profile, or the Default config dir.
_Avoid_: current config, selected profile

**Source**:
Why the Active profile is active: `env`, `dotfile` or `default`.
_Avoid_: origin, reason

**Dotfile**:
The `.agent-profile` file in a directory, mapping Agents to Profile names.
_Avoid_: local config

**Trust**:
Per-directory approval of a Dotfile, bound to the directory and the SHA-256 of the file's exact bytes. Verbs: trust, untrust.
_Avoid_: approve, grant, allowlist

### Skills

**Skill**:
A directory containing a regular `SKILL.md`.
_Avoid_: plugin, command

**Skill pool**:
A configured directory searched recursively for Skills.
_Avoid_: library, registry, repo

**Skill link**:
A symlink inside a Profile's `skills/` directory pointing at a Skill's source directory.
_Avoid_: installed skill, copied skill, entry

**Alias**:
A Skill link whose name differs from its source's basename (`--name`).
_Avoid_: rename, nickname

**Linked-skill access**:
An `additionalDirectories` entry that `ap` adds for the real directory behind a Skill link, removed only if `ap` added it.
_Avoid_: grant, allowlist

**Picker**:
The interactive `ap skills select` checklist for choosing which pool Skills a Profile links.
_Avoid_: wizard, selector

**External link**:
A Skill link whose target is outside every configured Skill pool.
_Avoid_: foreign link, unmanaged link

**Broken link**:
A Skill link whose target no longer exists.
_Avoid_: dangling link, stale link

**Plugin skill**:
A skill installed through the Agent's own `/plugin` mechanism. `ap` never manages these.
_Avoid_: skill (unqualified)

### Settings

**Seeded settings**:
Settings written once at `create --init` and owned by the user afterwards.
_Avoid_: default settings, template

**ap-owned settings**:
Settings that `update` keeps refreshing in existing Profiles (currently the status line).
_Avoid_: tool-owned

**Guardrails**:
The seeded `allow`, `ask` and `defaultMode` settings together with the managed deny rules.
_Avoid_: policy, safety settings

**Managed deny rules**:
Deny rules installed in the Agent's managed settings file so they apply to every Profile regardless of its config dir.
_Avoid_: global deny list

### Shell layer

**Shim**:
The `ap` shell function installed by `init`, which evaluates the shell code the binary prints. The binary it calls is `agent-profiles`; `ap` means the command you type.
_Avoid_: wrapper, alias

**Auto-switch hook**:
The chpwd (zsh) or PROMPT_COMMAND (bash) hook that applies a trusted Dotfile when you enter a directory.
_Avoid_: watcher, listener
