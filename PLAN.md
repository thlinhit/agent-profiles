# agent-profiles: plan

Working name, rename freely. A Go CLI that switches per-agent config profiles
(accounts, settings, MCP, skills) per shell and per directory. Clean
reimplementation inspired by `quinnjr/claude-code-profiles` (MIT). No code is
copied from it, so no license notice is required; credit it in the README anyway.

## Scope

v1:
- Agent: Claude Code only, behind an `Agent` adapter interface so other agents
  (Pi, Codex, Gemini) can be added without touching core code.
- Platforms: macOS and Linux, zsh and bash.
- Out of scope: Windows, PowerShell, cmd.exe, self-updater, skills sync.

## Core constraint

A child process cannot change its parent shell's environment. The binary
therefore never "switches" anything itself. It prints shell code, and a thin
shell function evals it:

```sh
# installed by: eval "$(agent-profiles init zsh)"
ap() { eval "$(command agent-profiles "$@" --emit-shell)"; }
```

Rule: the binary is the only place that parses untrusted input. The shell shim
only evals output that the binary has already validated and quoted.

## Architecture

```
cmd/agent-profiles/      main, cobra (or stdlib flag) commands
internal/profile/        store: list, create, delete, resolve; name validation
internal/agent/          Agent interface + registry
internal/agent/claude/   Claude Code adapter
internal/shell/          zsh/bash init scripts, safe quoting of emitted exports
internal/dotfile/        find and read .agent-profile up the tree; safe write
internal/trust/          per-directory approval store
```

### Agent adapter

```go
type Agent interface {
    Name() string                       // "claude"
    EnvVar() string                     // "CLAUDE_CONFIG_DIR"
    DefaultConfigDir() (string, error)  // e.g. ~/.claude
    InitTemplate() []byte               // optional settings.json skeleton
}
```

Adding an agent = one package + one registry line. The env var each future
agent uses must be verified from its own docs at the time, not assumed.

### Storage layout

```
$XDG_DATA_HOME/agent-profiles/      (0700)
  claude/
    work/                            (0700)  -> value of CLAUDE_CONFIG_DIR
    personal/
  trust.json                         (0600)
$XDG_CONFIG_HOME/agent-profiles/config.toml
```

Dotfile `.agent-profile` (TOML, so it can grow per-agent keys):

```toml
claude = "work"
```

## Commands (v1)

| Command | Effect |
|---|---|
| `init zsh\|bash` | Print the shim + optional chpwd/PROMPT_COMMAND hook |
| `use <agent> <name>` | Emit `export CLAUDE_CONFIG_DIR=...` for this shell |
| `off <agent>` | Emit `unset` |
| `current` | Show active profile per agent and its source (env, dotfile, default) |
| `list [agent]` | List profiles |
| `create <agent> <name> [--init]` | Create profile dir (0700), optional settings (0600) |
| `delete <agent> <name>` | Delete, with confirmation; never follow symlinks out of the profile dir |
| `local <agent> <name>` | Write `.agent-profile` in cwd (safe write) |
| `trust` / `untrust` | Approve or revoke the current directory's dotfile |

## Security requirements (from the upstream review, 2026-11-10)

1. **No injection via emitted shell code.** All emitted values are
   single-quote escaped by one function in `internal/shell`, fuzz-tested with
   quotes, `$`, backticks, `;`, `&`, newlines, and non-UTF-8 bytes.
2. **Strict names.** `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`. Checked before any
   filesystem or output use.
3. **Safe dotfile write.** Write to a temp file in the same dir, then
   `os.Rename` over the path. Rename replaces the entry, it does not follow a
   symlink. Refuse if the existing path is a symlink and say why.
4. **Private permissions.** `os.MkdirAll(dir, 0o700)`, settings via
   `os.OpenFile(..., O_CREATE|O_EXCL, 0o600)`. Do not chmod user-supplied
   ancestors.
5. **Per-directory trust.** A dotfile selects a profile only after
   `trust` for that directory, keyed by absolute path + dotfile SHA-256.
   Content change requires re-trust. Untrusted dotfile: print a one-line hint,
   do nothing.
6. **Dotfile reading.** Size cap (4 KiB), reject symlinks pointing outside
   the repo root, stop walking at `$HOME` or the filesystem root.

## Milestones

| # | Deliverable | Done when |
|---|---|---|
| M0 | Repo, go.mod, CI (go test, go vet, golangci-lint), goreleaser config | CI green on empty skeleton |
| M1 | `profile` store + name validation + `create/list/delete` | Unit tests incl. permission bits under umask 022 |
| M2 | `shell` quoting + `init` + `use/off/current` | Fuzz test passes; manual zsh and bash switch works |
| M3 | `dotfile` find/read/safe write + `local` | Symlink-clobber regression test passes |
| M4 | `trust` + auto-switch hook | Untrusted repo cannot select a profile; trusted one does |
| M5 | Release: goreleaser binaries (darwin/linux, amd64/arm64), Homebrew tap, README | `brew install` and `eval "$(agent-profiles init zsh)"` works on a clean machine |
| M6 | Second adapter (Pi or Codex) | Added with no changes outside its package + registry |

M6 is the real test of the design. If it needs core changes, fix the
interface before adding a third agent.

## Testing

- Unit: table tests per package; `t.TempDir()` with fake HOME and XDG vars.
- Fuzz: `go test -fuzz` on shell quoting and dotfile parsing.
- Integration: run the emitted shim in real `zsh -fc` / `bash --norc -c` and
  assert the resulting env.
- Security regressions: one test per requirement above, named after it.

## Open questions

- Migration: import existing upstream profiles from
  `$XDG_DATA_HOME/claude-code-profiles/`? (Needs checking where upstream stores them.)
- Should `use` without an agent apply to all agents configured in the dotfile?
- Final name and Go module path.
