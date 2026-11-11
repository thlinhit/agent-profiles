# Local usage, M0–M4

Build with Go 1.25 or newer:

```sh
go build -o agent-profiles ./cmd/agent-profiles
```

Put the binary on PATH, then initialize the current shell:

```sh
eval "$(agent-profiles init bash)" # or zsh
ap create claude work --init
ap use claude work
ap current
ap list claude
ap off claude
```

The binary prints exports and unsets. Only the `ap` function changes its calling
shell. Informational output goes to stderr when invoked through `ap`, so it is
never evaluated. Failed commands do not evaluate any output.

```sh
ap local claude work
ap trust
# Edit .agent-profile later? Run ap trust again to approve the new content.
ap untrust
ap delete claude work --yes
```

`local` requires an existing profile and writes in the current directory.
`trust` approves only a dotfile in the current directory, not an ancestor's.
Deletion without `--yes` prompts on terminal stdin with `[y/N]`. Only `y` or
`yes`, case-insensitive, proceeds; an empty answer or any other input cancels.
Non-terminal stdin is refused unless `--yes` is supplied. Prompts go to stderr,
including when deleting through the `ap` shell function.

Initialization immediately checks the current directory. Zsh checks again on
`chpwd`; bash checks before each prompt. Existing hooks are retained, repeated
initialization does not add duplicate hooks, and the auto hook preserves the
previous exit status. The internal `auto` command can also be called manually.

Only an approved dotfile can cause automatic exports. An untrusted or changed
dotfile prints a one-line hint and emits nothing. Malformed files cause an error
and emit nothing. A missing dotfile also emits nothing. Existing environment
values therefore remain in place when leaving a trusted directory or entering
an untrusted one; use `ap off claude` outside a trusted selection to clear one.
Inside a trusted directory the next hook will reapply that selection.

`current` reports environment values first, then a trusted dotfile selection,
then the adapter's default directory. Profiles use
`$XDG_DATA_HOME/agent-profiles/claude/<name>`, or
`$HOME/.local/share/agent-profiles/claude/<name>` when XDG_DATA_HOME is unset.
Relative XDG_DATA_HOME values are rejected. Application-owned directories must
be private; existing permissive directories are rejected rather than chmodded.
User-supplied ancestors are never chmodded. No user config file is needed yet.

Dotfiles are TOML maps of agent names to profile names, capped at 4 KiB.
Search includes the nearest repository root or HOME boundary, never going above
it. Without a repository marker it stops at HOME, or the filesystem root if
HOME is not an ancestor. Symlinks are allowed for reading only when their
resolved file stays inside that boundary. Writes always refuse symlink entries.
Trust binds the canonical absolute directory to the SHA-256 of the exact bytes
read. The private JSON database uses atomic replacement and an advisory lock
for concurrent writers. No dotfile can execute commands.

## Security regression map

| PLAN.md requirement | Regression |
| --- | --- |
| No injection via emitted shell code | `TestNoInjectionViaEmittedShellCode`, `FuzzQuote` |
| Strict names | `TestStrictNames` |
| Safe dotfile write | `TestSafeDotfileWrite` |
| Private permissions | `TestPrivatePermissions` |
| Per-directory trust | `TestPerDirectoryTrust`, `TestAutoSwitchRequiresPerDirectoryTrust` |
| Dotfile reading | `TestDotfileReading`, `FuzzParse` |

Integration tests run real bash and zsh shells. An unavailable shell is reported
as skipped. CI runs tests, vet and golangci-lint on macOS and Linux. No CI run is
triggered by local commits. GoReleaser configuration is present but publishing
is disabled; no releases or Homebrew integration are included.

This is a clean reimplementation inspired by
[quinnjr/claude-code-profiles](https://github.com/quinnjr/claude-code-profiles).
No upstream code was copied. A second adapter and release work are out of scope.
