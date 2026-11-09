# laziergit

A proof of concept: [lazygit](https://github.com/jesseduffield/lazygit)'s
functionality in a native desktop app, drawn on the GPU by
[MyGo](https://github.com/egoist/mygo). No webview, no terminal.

lazygit's git layer (`pkg/commands`) is imported as a Go library, so the
git behaviour (loaders, staging, interactive rebase through lazygit's
daemon, push and pull with credential prompts) is lazygit's own. Only the UI
is new.

## Install

With Homebrew, on macOS 12 or later:

```sh
brew install Siemko/tap/laziergit
laziergit            # in a repository, or:
ln -sf "$(brew --prefix)/opt/laziergit/laziergit.app" /Applications/laziergit.app
```

With Go 1.27.3+ (a `laziergit` command, without the app bundle):

```sh
go install github.com/Siemko/laziergit@latest
```

The app isn't signed with a Developer ID yet, so macOS refuses to open the
app of a disk image from the [releases](https://github.com/Siemko/laziergit/releases)
downloaded with a browser. Homebrew and `go install` don't mark what they
install as downloaded, so neither is refused. To use a disk image anyway,
check it against the release's `SHA256SUMS` first (`shasum -a 256 -c
SHA256SUMS --ignore-missing`), and only then clear the mark of the app you
copied: `xattr -dr com.apple.quarantine /Applications/laziergit.app`.

## Run from source

Requires Go 1.27.3+.

```sh
go run . [path/to/repo…]      # or: go tool mygo dev
go test ./...                 # the UI tests run headless, without a window
go tool mygo build            # build/darwin-arm64/laziergit.app and a .dmg
```

`LAZIERGIT_SNAPSHOTS=<dir> go test ./internal/app` saves PNGs of the views
the tests drive. `LAZIERGIT_CAPTURE=<file.png>` (with an optional
`LAZIERGIT_CAPTURE_SECTION=history`) makes the app save a picture of its
window, then quit.

## What works

| Area | |
|---|---|
| Changes | file list with status, stage/unstage a file or everything, discard, commit (summary and description), amend |
| Staging | lazygit's line staging: the file's diff line by line, syntax highlighted; choose lines (click, ⇧-click, `j`/`k`, `J`/`K`, `v` range, `a` hunk) and stage, unstage or discard them (`space`, `d`), or a whole hunk from its header; `tab` switches between the staged and unstaged sides |
| History | the latest 300 commits, with lazygit's graph drawn as curves, branch and tag chips, pushed/unpushed state; commit files and per-file diff; **reword, fixup, move up/down, drop** (interactive rebase); continue/abort banner |
| Conflicts | each conflict as a card of its current, base (diff3) and incoming sides, highlighted, the lines between folded; keep a side, both, or every conflict's one side (`space`, `b`, `j`/`k`, `←`/`→`), undo (`z`); the file is staged once resolved, and the app offers to continue; Abort / Skip / Continue for merges, rebases, cherry-picks and reverts, started in the app or not |
| Branches | checkout, new, merge, rebase onto, delete; upstream ahead/behind; each branch's commits, with their graph |
| Cherry-pick | copy commits from any branch's log or the history (`C`), paste them onto the current branch (`V`), conflicts resolved as a merge's |
| Undo / redo | lazygit's reflog undo (`z`, `Z`), interoperable with lazygit's: commits, resets, pulls, checkouts and rebases, and also cherry-picks, reverts and merges; a hard reset stashes local changes around it |
| Stash | stash all, pop, apply, drop; files and diff |
| Remotes | fetch, pull, push (sets the upstream on first push), force-push with lease; username/password/passphrase prompts |
| Windows | a window per repository (⌘N, ⌘O, File → Open Recent, the Window menu); opening one already open brings its window to the front; `laziergit <path>` from a terminal opens it in the app running; the windows of the last session open again; each window keeps its place |
| App | vibrant sidebar, ⌘K command palette, lazygit's single-key bindings (`1`–`4`, `j`/`k`, `space`, `a`, `c`, `d`, `r`, `f`, `p`, `P`, `s`, `g`, `n`, `M`, `⌃j`/`⌃k`), status-bar key hints, context menus, toasts, command log (⌘L), auto-refresh on disk changes, light and dark |

## Layout

```
tools/genicon       draws resources/icon.png
scripts/            the third-party notices, and the Homebrew formula of a release
main.go             startup: lazygit's rebase daemon, the login shell's PATH, a single instance
windows.go          the windows, their menus, recent repositories and the session
internal/core       a thin adapter over lazygit's GitCommand: Open, Load (a Snapshot), actions
internal/core/graph lazygit's commit-graph layout, kept as geometry (its pipes are unexported upstream)
internal/app        the MyGo views: sidebar, lists, diffs, dialogs, the action registry
```

Every command is declared once, in `internal/app/actions.go`. The
palette, the keyboard shortcuts and the status-bar hints all come from that
registry.

## Not yet

These are from lazygit, still to port:
- custom patches
- bisect
- worktrees and submodules
- custom commands
- filtering and diffing modes
- reflog
- remote-branch and tag panels
- settings

Limitations:
- lazygit assumes one repository per process: it changes the working
  directory and git's environment, and some of its file operations go by
  relative paths. Windows' repositories take turns (`core.Repo.enter`), so a
  long push in one window holds back the others' git commands until it ends.
- It uses lazygit's config format, kept in `~/Library/Application Support/laziergit`.

Started from Finder or the Dock, the app takes its `PATH` from your login
shell, so that git finds Homebrew's tools (gpg, git-lfs, credential helpers)
and hooks find theirs. `LAZIERGIT_DEBUG=1` logs what the windows do.

## Releasing

Bump `version` in `mygo.json`, run `scripts/notices.sh`, commit, and push the
tag `v<version>`: the release workflow tests, builds and publishes the app.
Then `scripts/formula.sh <version>` prints the tap's formula.

## License

MIT. The modules laziergit includes are under their own licenses, collected
in `resources/THIRD_PARTY_NOTICES.md`.
