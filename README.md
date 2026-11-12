# agent-profiles

Switch config profiles for AI coding agents (accounts, settings, MCP servers,
skills) per shell and per directory.

Supported agents:

| Agent | Environment variable | Default config dir |
| --- | --- | --- |
| `claude` (Claude Code) | `CLAUDE_CONFIG_DIR` | `~/.claude` |
| `pi` (Pi coding agent) | `PI_CODING_AGENT_DIR` | `~/.pi/agent` |

Shells: zsh and bash.

## How it works

A child process cannot change its parent shell's environment, so the binary
never switches anything itself. It prints shell code, and a small `ap` shell
function evaluates it. The binary is the only component that parses input; the
shell function only evaluates output the binary has already validated and
quoted.

## Install

Requires Go 1.25 or newer.

```sh
go install github.com/thlinhit/agent-profiles/cmd/agent-profiles@latest
```

Or from a local clone:

```sh
go install ./cmd/agent-profiles
```

Both install into `$(go env GOPATH)/bin` (usually `~/go/bin`). Make sure that
directory is on your `PATH`:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

Then add to your `~/.zshrc` or `~/.bashrc`, after the `PATH` line:

```sh
eval "$(agent-profiles init zsh)"   # or: init bash
```

## Quick start

```sh
ap create claude work --init   # create a profile
ap use claude work             # switch this shell to it
ap current                     # show active profile per agent
ap list                        # list all profiles
ap off claude                  # back to the default config dir
```

### Per-directory profiles

```sh
cd ~/code/work-repo
ap local claude work   # writes .agent-profile in this directory
ap trust               # approve it; required before it takes effect
```

From then on, entering the directory switches automatically (zsh on `chpwd`,
bash before each prompt). If `.agent-profile` changes, run `ap trust` again.
`ap untrust` revokes approval.

## Shared Claude guardrails

`ap create claude <name> --init` seeds `allow`, `ask`, and `defaultMode` once.
It also seeds a status line showing profile, model, context used, and directory with git branch and dirty marker (`*`); requires `jq` and makes no API calls.
`ap update claude <name>` (or `--all`) refreshes the status line in existing profiles and leaves `permissions` and `env` untouched.
Profiles may edit these settings afterwards. Deny rules live in managed settings
so every profile gets them regardless of `CLAUDE_CONFIG_DIR`.

Install the shared deny rules on macOS from this repository:

```sh
sudo install -m 644 -o root managed/claude/managed-settings.json "/Library/Application Support/ClaudeCode/"
```

To roll back, use `git revert` on the guardrail change and reinstall, or remove
the installed file:

```sh
sudo rm "/Library/Application Support/ClaudeCode/managed-settings.json"
```

## Commands

| Command | Description |
| --- | --- |
| `init zsh\|bash` | Print the shell function and auto-switch hook |
| `use <agent> <name>` | Activate a profile in the current shell |
| `off <agent>` | Unset the agent's profile variable |
| `current` | Show the active config dir per agent and its source (env, dotfile, default) |
| `list [agent]` | List profiles |
| `create <agent> <name> [--init \| --from <profile>] [--base-url <url>] [--auth-token-stdin]` | Create a profile; `--init` writes a starter config; `--from` copies only `settings.json` from an existing profile (no login, MCP servers, sessions or history); `--base-url` (claude only) sets `env.ANTHROPIC_BASE_URL`, e.g. for a local proxy. Only `http(s)` URLs without credentials or query are accepted; `--auth-token-stdin` (claude only) sets `env.ANTHROPIC_AUTH_TOKEN`, prompting without echo on a terminal or reading one line from a pipe, so the token never appears in argv or shell history |
| `delete <agent> <name> [--yes]` | Delete a profile; prompts unless `--yes` |
| `exec <agent> <name> [-- <cmd> [args...]]` | Run one command with a profile selected, leaving the current shell unchanged. Without `--` it runs the agent itself (`ap exec claude work` runs `claude`). Arguments are passed verbatim, never through a shell |
| `local <agent> <name>` | Write the selection to `.agent-profile` in the current directory |
| `trust` / `untrust` | Approve or revoke the current directory's `.agent-profile` |

Profiles live in `$XDG_DATA_HOME/agent-profiles/<agent>/<name>`
(default `~/.local/share/agent-profiles/<agent>/<name>`).

## Security

- A `.agent-profile` file has no effect until you `ap trust` it. Trust is bound
  to the directory and the SHA-256 of the file's exact bytes, so any edit
  requires re-approval. Untrusted files are never parsed.
- Dotfiles are plain TOML maps (agent to profile name), capped at 4 KiB, and
  cannot execute commands.
- All emitted shell code is quoted; injection is covered by tests and fuzzing.
- Profile and trust directories must be private; permissive ones are rejected.

## Behavior details

**Shell output.** Through `ap`, informational output goes to stderr so it is
never evaluated. Failed commands evaluate nothing. `init` checks the current
directory immediately, keeps existing hooks, never adds duplicates, and the
auto hook preserves the previous exit status.

**Automatic selection.** For `auto` and `current`, filesystem and size checks
run first, then the raw-byte hash is checked against trust, and only trusted
bytes reach the TOML parser. An untrusted or changed dotfile prints a one-line
hint. A trusted but malformed file is an error and emits nothing; so does a
missing dotfile. Existing environment values therefore stay in place when
leaving a trusted directory or entering an untrusted one. Use `ap off <agent>`
to clear them.

**`current`** reports, in order of precedence: the environment variable, a
trusted dotfile selection, then the agent's default directory.

**`local` and `trust`.** `local` requires an existing profile and merges with
existing entries in the dotfile. `trust` approves only a dotfile in the current
directory, not an ancestor's, and does not validate it; validation happens on
the next trusted resolution.

**`delete`** without `--yes` prompts `[y/N]` on terminal stdin. Only `y` or
`yes` (case-insensitive) proceeds. Non-terminal stdin is refused without
`--yes`.

**Dotfile search** stops at the nearest repository root or `$HOME`, never
above it; with neither, it stops at the filesystem root. Symlinks are followed
for reading only when they resolve inside that boundary. Writes refuse
symlinks.

**Storage.** Relative `XDG_DATA_HOME` values are rejected. Directories the tool
owns must be private: existing permissive ones are rejected, not chmodded, and
user-supplied ancestors are never chmodded. The trust database is private JSON
written with atomic replacement and an advisory lock.

## Security regression tests

| Requirement | Tests |
| --- | --- |
| No injection via emitted shell code | `TestNoInjectionViaEmittedShellCode`, `FuzzQuote` |
| Strict names | `TestStrictNames` |
| Safe dotfile write | `TestSafeDotfileWrite` |
| Private permissions | `TestPrivatePermissions` |
| Per-directory trust | `TestPerDirectoryTrust`, `TestAutoSwitchRequiresPerDirectoryTrust`, `TestUntrustedDotfileIsNeverParsed` |
| Dotfile reading | `TestDotfileReading`, `FuzzParse` |

## Development

```sh
go test ./...
golangci-lint run
golangci-lint fmt   # fix import grouping
```

Integration tests run real bash and zsh shells; a missing shell is skipped.

## Credits

A clean reimplementation inspired by
[quinnjr/claude-code-profiles](https://github.com/quinnjr/claude-code-profiles).
No upstream code was copied.
