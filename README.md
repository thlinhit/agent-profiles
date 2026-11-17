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

Requires Go 1.26.11 or newer.

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
ap trust               # trust it; required before it takes effect
```

From then on, entering the directory switches automatically (zsh on `chpwd`,
bash before the next prompt only when `$PWD` changes). Both run once at shell
initialization. In bash, `ap local`, `ap trust` and `ap untrust` also refresh the
selection at the next prompt; `ap auto` refreshes it explicitly without changing
directories. If `.agent-profile` changes, run `ap trust` again.
`ap untrust` revokes approval.

## Skills

Link skills into Claude or Pi profiles without copying them:

```sh
ap skills add claude work ~/my-repos/skills/code-review
ap skills add pi work ~/my-repos/skills/code-review --name review
ap skills ls claude work
ap skills rm claude work code-review
ap skills ls --pool
ap skills select claude work           # interactive picker over pool skills
ap skills add claude work code-review  # look up a bare name in configured pools
```

Manage pools with:

```sh
ap skills pool add ~/my-repos/AI-collaboration
ap skills pool ls
ap skills pool rm ~/my-repos/AI-collaboration
```

Add requires an existing directory and stores its absolute path. Adding the same
pool again is a no-op. Removal accepts `~/` or absolute paths, even for missing
pools. Pool listings mark paths that no longer exist with `(missing)`.

You can also edit `$XDG_CONFIG_HOME/agent-profiles/config.toml`
(default `~/.config/agent-profiles/config.toml`) directly:

```toml
skill_pools = ["~/my-repos/AI-collaboration"]
```

Pool lookup searches recursively, skipping `.git`, `node_modules`, and symlinked
directories. Duplicate names require a full path. `ls --pool` lists available
skills sorted by name.

Sources must be directories containing a regular `SKILL.md`, outside profile
storage and the temporary directory (`$TMPDIR` when set). Links use absolute,
resolved source paths. Names use the source basename unless `--name` is given.
Existing entries are never replaced: run `skills rm` first. Removal deletes
only the symlink, never its target; real files and directories must be deleted
manually. A symlinked `skills/` directory is refused. Listings mark broken links
and entries that are not links. Plugin skills are separate: manage them with
`/plugin` inside the Claude profile.

### Interactive picker

`ap skills select [<agent> [<profile>]]` opens a checklist of every pool skill,
with linked skills pre-ticked. Missing agent or profile arguments are prompted.
Keys: space or x toggles, `/` filters by label, ctrl+a toggles all shown, enter
continues, ctrl+c aborts. A confirm page shows the `+`/`-` diff; answering No
returns to the list with selections kept.

- Each row is one link, so aliases made with `--name` show as
  `review → code-review (alias)`. External and broken links are listed and can
  be unticked. Real directories are skipped with a note.
- Two ticked skills with the same name are rejected before confirm.
- Every add is validated before anything changes; then removes run before
  adds, best effort, with one result line per skill and a non-zero exit on any
  failure. A failed swap prints the command to restore the old link.
- An unreadable pool is skipped with a warning instead of failing the picker.
- Requires a terminal on stdin and stderr (the UI draws on stderr because the
  `ap` shell function captures stdout); scripts should use `add`/`rm`.

## MCP servers

An **MCP pool** is one file of named **Pool servers**. `ap mcp sync` copies every
Pool server into every profile of every Agent that supports MCP (Claude and Pi),
as **Managed servers**. The pool is `$XDG_CONFIG_HOME/agent-profiles/mcp.json`
(default `~/.config/agent-profiles/mcp.json`). To keep it elsewhere, set `mcp_pool`
in `config.toml`:

```toml
mcp_pool = "~/dotfiles/mcp.json"
```

```sh
ap mcp add-server docs --env LOG=debug -- npx -y docs-mcp
ap mcp add-server atlassian --url https://mcp.atlassian.com/v1/mcp --auth oauth
ap mcp list
ap mcp sync
ap mcp remove-server docs
```

`add-server` and `remove-server` change only the pool and print `pool: + atlassian`
or `pool: - docs`. Run `ap mcp sync` to apply the change to profiles. `sync` prints one
line per change, for example:

```text
mcporter: updated
claude/work: + atlassian
pi/work: + atlassian
```

A profile with nothing to change prints `claude/work: unchanged`.

- Sync resets each Managed server to the pool's version: hand edits are
  overwritten, and hand deletions are added back (`~` or `+`).
- Servers you wrote yourself with a different name are never touched. If one
  has the same name as a Pool server, the pool's version replaces it. The old
  definition is saved to `.ap-mcp-backup.json` in that profile, and sync prints
  `! atlassian replaced by pool; backup saved`.
- Sync records its Managed servers in `.ap-mcp.json` in the profile directory.
  A Managed server is removed from profiles when its name leaves the pool.
- A Pool server that an Agent cannot run is skipped with a `!` line. Its existing
  Managed server stays in place.
- `ap` writes its own config only when the content changes, and it never edits
  a profile's other keys.

**Secrets** go in `${VAR}` references, never in literal values. Quote the value so
your shell does not expand it first:

```sh
ap mcp add-server docs --url https://docs.example.com/mcp --header 'Authorization=Bearer ${DOCS_TOKEN}'
```

`ap` stores the text as typed and warns for any `--header` or `--env` value that
has no `${...}` reference. The warning names the key, never the value.

### Shared login

`--auth oauth` marks a Pool server that uses OAuth. Every profile of both Agents
uses one **Shared login** for it, so you log in once:

```sh
ap mcp login atlassian    # runs mcporter's browser login in this terminal
ap mcp list               # atlassian  oauth  logged in (expires 2026-12-01 09:00)
```

- Requires mcporter 0.17.0 or newer and Node 24 or newer. Without them, an OAuth
  server is skipped with a reason such as `! atlassian skipped: mcporter not found
  (needs mcporter >= 0.17.0 and Node >= 24)`. Direct servers still sync, and a
  profile that already has the server keeps it. `ap mcp list` then shows
  `unavailable`.
- `ap mcp login` checks the mcporter daemon first. It starts a stopped daemon. If
  leftover `mcporter serve` processes from old sessions still hold Pool servers, it
  asks once before stopping them; anything but `y` stops nothing. If a crashed daemon left stale daemon metadata behind, `ap` asks once (or, after the `y` above, clears it without another question) by moving `user.json` into a new `user.json.stale-<pid>-*` directory beside it. mcporter removes the leftover socket itself when it starts. `ap mcp list` shows
  `(mcporter daemon not running)` on OAuth rows while the daemon is down.
- Tokens are stored in `$XDG_DATA_HOME/mcporter/credentials.json` (falling back to
  `~/.mcporter/credentials.json`), outside every profile. `ap` writes its mcporter
  config to `$XDG_DATA_HOME/agent-profiles/mcporter.json` (default
  `~/.local/share/agent-profiles/mcporter.json`).
- Set `MCPORTER_VAULT_PASSWORD` in your shell for mcporter. `ap` never reads or
  stores it.
- Point plain `mcporter` at ap's config, so it sees only the Pool and not the
  servers in `~/.claude.json`, Claude Desktop, Codex or VS Code:

  ```sh
  # ~/.zshrc or ~/.bashrc
  export MCPORTER_CONFIG="$HOME/.local/share/agent-profiles/mcporter.json"
  ```

  mcporter stores a login by server name. If another config defines a server
  with the same name but a different URL, running plain `mcporter list` can wipe
  the Shared login. Keep each OAuth server only in the Pool.
- Claude tools from an OAuth server get a prefixed name, for example
  `mcp__atlassian__atlassian__<tool>`.
- `ap mcp list` reads only the credential metadata. It never prints a token.

### Syncing

- `ap update` syncs MCP for Claude only. Run `ap mcp sync` to sync Pi profiles.
- Run `ap mcp sync` with Claude sessions closed. Claude rewrites `.claude.json`
  while it runs.
- Restart running sessions to load a change.

### Limits

- The Agent runs as you, so the Managed deny rules cannot fully protect mcporter's
  credentials. Anything the Agent runs can still read them.
- Pi has no persistent deny rule, so mcporter's credentials are protected for
  Claude only.
- External MCP servers are left alone: servers from claude.ai, and servers in a
  project's `.mcp.json`. The Agent's own approval rules apply to them.
- A direct HTTP server with its own OAuth login, not `--auth oauth`, logs in
  separately for each profile through the Agent.
- One Shared login covers each OAuth server for every profile. Two accounts for
  the same OAuth server are not supported.

## Shared Claude guardrails

`ap create claude <name> --init` seeds `allow`, `ask`, and `defaultMode` once.
It also seeds `attribution.commit: ""`, which hides the `Co-Authored-By` trailer on commits (the older `includeCoAuthoredBy` key is deprecated upstream).
It also seeds a status line showing the profile (with an emoji picked from its name), model, context used, and directory with git branch and dirty marker (`*`); requires `jq` and makes no API calls.
`ap update claude <name>` (or `--all`) refreshes the status line in existing profiles and leaves `permissions` and `env` untouched.
Profiles may edit these settings afterwards. Deny rules live in managed settings
so every profile gets them regardless of `CLAUDE_CONFIG_DIR`. The same file also
denies `mcporter` commands and reads of its credentials; see [MCP servers](#mcp-servers).

Install the shared deny rules on macOS from this repository:

```sh
sudo install -m 644 -o root managed/claude/managed-settings.json "/Library/Application Support/ClaudeCode/"
```

To roll back, use `git revert` on the guardrail change and reinstall, or remove
the installed file:

```sh
sudo rm "/Library/Application Support/ClaudeCode/managed-settings.json"
```

## Global instructions

Claude Code also loads instruction files from your home directory (`~/.claude/CLAUDE.md`, `~/.claude/rules/**`, `~/AGENTS.md`, `~/.claude/AGENTS.md`, `~/CLAUDE.md`, `~/CLAUDE.local.md`), even when `CLAUDE_CONFIG_DIR` points at a profile. Each claude profile has Global instructions **on** (the default) or **off**.

- Off writes those six paths into `claudeMdExcludes` in the profile's `settings.json`. Entries you added yourself are kept.
- On removes only those six entries, and drops `claudeMdExcludes` if it ends empty.
- The state is read from `settings.json`; there is no separate record. A profile edited by hand into neither state reports `partial`.
- `ap create claude <name>` asks on a terminal; `--global-instructions on|off` sets it without asking, and `--from` keeps the source profile's setting.
- Restart running Claude sessions after a change.

```sh
ap settings set claude work global-instructions off
ap settings get claude work global-instructions   # on | off | partial
ap settings                                      # interactive, in a terminal
```

## Commands

| Command | Description |
| --- | --- |
| `create <agent> <name> [--init \| --from <profile>] [--base-url <url>] [--auth-token-stdin] [--global-instructions on\|off]` | Create a profile; `--init` writes a starter config; `--global-instructions` (claude only) sets [Global instructions](#global-instructions) and skips the question; `--from` copies only `settings.json` from an existing profile (no login, MCP servers, sessions or history); `--base-url` (claude only) sets `env.ANTHROPIC_BASE_URL`, e.g. for a local proxy. Only `http(s)` URLs without credentials or query are accepted; `--auth-token-stdin` (claude only) sets `env.ANTHROPIC_AUTH_TOKEN`, prompting without echo on a terminal or reading one line from a pipe, so the token never appears in argv or shell history |
| `list [agent] [--json]` | List profiles; JSON output includes agent, profile, and path |
| `update <agent> <name> \| --all` | Refresh ap-owned settings in one or all profiles |
| `settings [<agent> [<profile>]]` | Settings screen: Space/Enter toggles and saves at once, Esc closes; needs a terminal |
| `settings ls <agent> <profile> [--json]` | List Profile settings and their values |
| `settings get <agent> <profile> <key> [--json]` | Print one Profile setting: `on`, `off` or `partial` |
| `settings set <agent> <profile> <key> <on\|off>` | Change a Profile setting; only `on` and `off` are accepted. Known keys: `global-instructions` |
| `delete <agent> <name> [--yes]` | Delete a profile; prompts unless `--yes`, which is required without terminal stdin |
| `use <agent> <name>` | Activate a profile in the current shell |
| `off <agent>` | Deactivate a profile in the current shell |
| `current [--json]` | Show the Active profile per Agent and its Source (env, dotfile, default) |
| `exec <agent> <name> [-- <cmd> [args...]]` | Run one command with a profile selected, leaving the current shell unchanged. Without `--` it runs the agent itself (`ap exec claude work` runs `claude`). Arguments are passed verbatim, never through a shell |
| `local <agent> <name>` | Write the Selection to `.agent-profile` in the current directory |
| `trust` / `untrust` | Trust or untrust the current directory's `.agent-profile` |
| `skills ls <agent> <profile> [--json]` / `skills ls --pool [--json]` | List profile skills or available pool skills |
| `skills add <agent> <profile> <path-or-name> [--name N]` | Link a Skill into a profile |
| `skills rm <agent> <profile> <skill>` | Remove only a Skill link |
| `skills select [<agent> [<profile>]]` | Interactively choose pool Skills; prompts in a terminal, use `ap skills add/rm` instead without one |
| `skills pool add <path>` / `skills pool rm <path>` / `skills pool ls [--json]` | Manage and list Skill pools |
| `mcp sync` | Write ap's mcporter config and sync Managed servers into every profile of every MCP-capable Agent |
| `mcp list` | List Pool servers; OAuth servers show Shared login status |
| `mcp add-server <name> (--url <url> \| -- <cmd> [args...]) [--auth oauth] [--header K=V]... [--env K=V]...` | Add a Pool server to the MCP pool; run `mcp sync` to apply |
| `mcp remove-server <name>` | Remove a Pool server from the MCP pool; run `mcp sync` to apply |
| `mcp login <name>` | Log in an OAuth Pool server through mcporter (Shared login) |
| `init zsh\|bash` | Print the shell function and Auto-switch hook |

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

**`current`** reports the environment variable when it is set, with source
`dotfile` if it equals the trusted Selection's path (after cleaning) and `env`
otherwise. When the variable is unset it reports the Default config dir with
source `default`, even if a trusted Dotfile exists in the directory.

**`local` and `trust`.** `local` requires an existing profile and merges with
existing entries in the dotfile. `trust` applies only to a dotfile in the current
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

The `ap` version defaults to `dev`. To set it at build time, pass the `main.version`
variable through `-ldflags`:

```sh
go build -ldflags "-X main.version=1.2.3" ./cmd/agent-profiles
```

```sh
go test ./...
golangci-lint run
golangci-lint fmt   # fix import grouping
```

Integration tests run real bash and zsh shells; a missing shell is skipped.

### Pre-push check

A git `pre-push` hook runs `make check` (vet, tests, and lint when `golangci-lint`
is installed) before every push. It applies to anyone running `git push` in this
repo, including Claude and Pi sessions. Enable it once per clone:

```sh
make hooks
```

Bypass in an emergency with `git push --no-verify`.

## Credits

A clean reimplementation inspired by
[quinnjr/claude-code-profiles](https://github.com/quinnjr/claude-code-profiles).
No upstream code was copied.
