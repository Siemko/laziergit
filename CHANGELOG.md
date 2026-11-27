# Changelog

laziergit's versions, newest first. Each version's section is its release
notes: the app shows them when it offers the update.

## 0.6.0

- laziergit updates itself: it checks for a new version in the
  background, shows what's new, and installs it when you say so.
  laziergit → Check for Updates… checks now. Installed with Homebrew, it
  leaves updating to `brew upgrade laziergit`.
- Faster in large repositories: a refresh reads only the reflog entries
  it shows, rather than the whole reflog, which a repository used for
  years has tens of thousands of (a third of a refresh's time, in one
  with 20,000).

## 0.5.2

- Commits copied to cherry-pick (C) are shared by the windows of a
  repository and of its worktrees: copy in one worktree's window, paste
  (V) in another's. The banner says the branch and the worktree they came
  from.
- A paste keeps the commits copied meanwhile, even in another window;
  uncopying works from whichever log a commit was copied from.
- Pasting applies a parent before its child, even after its branch got
  new commits since it was copied.

## 0.5.1

- The sidebar's sections go in the order of their keys, 1 to 9 from the
  top, and each key sits centred in its keycap. Tags, Reflog, Worktrees
  and Submodules are now 5 to 8, the first remote 9.

## 0.5.0

- The sidebar shows each section's key.
- Settings (⌘,): the theme, the status bar's key hints, the sidebar's
  shortcuts, reopening the last session; the diffs' layout and lines of
  context, how many commits History loads; fetching in the background,
  asking before a push, signing off commits, the main branches.

## 0.4.0

- History: choose commits with the mouse. Click, Shift-click, Cmd-click,
  or drag over the rows; a bar under the list copies, fixes up, moves or
  drops them; double-click rewords. The rows chosen show as one block.
- New sections: each remote's branches, tags, the reflog, worktrees and
  submodules.

## 0.3.0

- History: choose a range of commits (Shift with the keys, a Shift-click,
  or v) to fix up, move, drop or copy at once.
- Safety: discarding, stashing, undo and push check again just before
  they act, and refuse what would lose work.

## 0.2.0

- A polished design, in light and dark; layouts that fit narrow windows.

## 0.1.0

- lazygit's functionality as a native Mac app: changes and line staging,
  history with interactive rebase, branches, conflicts, cherry-pick, undo
  and redo, stash, push and pull. A window for each repository.
