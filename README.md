# laziergit

A proof of concept: [lazygit](https://github.com/jesseduffield/lazygit)'s
functionality in a native desktop app, drawn on the GPU by
[MyGo](https://github.com/egoist/mygo). No webview, no terminal.

lazygit's git layer (`pkg/commands`) is imported as a Go library, so the
git behaviour (loaders, staging, interactive rebase through lazygit's
daemon, push and pull with credential prompts) is lazygit's own. Only the UI
is new.

## Run

Requires Go 1.27.3+.

```sh
go run . [path/to/repo]       # or: go tool mygo dev
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
| App | vibrant sidebar, ⌘K command palette, lazygit's single-key bindings (`1`–`4`, `j`/`k`, `space`, `a`, `c`, `d`, `r`, `f`, `p`, `P`, `s`, `g`, `n`, `M`, `⌃j`/`⌃k`), status-bar key hints, context menus, toasts, command log (⌘L), auto-refresh on disk changes, light and dark |

## Layout

```
main.go             the window; runs lazygit's rebase daemon when git starts it
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

Two limitations in this proof of concept:
- One repository per process, because lazygit changes the working directory.
- It uses lazygit's config format, kept in `~/Library/Application Support/laziergit`.
