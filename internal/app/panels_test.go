package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// answer checks the prompt asks title, gives it value, and clicks button.
func answer(t *testing.T, a *App, tt *ui.Tester, title, value, button string) {
	t.Helper()
	if a.dialog == nil || a.dialog.title != title {
		t.Fatalf("dialog %+v, want %q", a.dialog, title)
	}
	a.dialog.value = value
	tt.Frame()
	if err := tt.Click(button); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

// panelsRepo is linear's repository with a bare remote, origin, which has
// main and a branch feature, and a tag v1 on three.
func panelsRepo(t *testing.T) (*App, *ui.Tester, string, string) {
	a, tt, dir := linear(t)
	bare := filepath.Join(t.TempDir(), "origin.git")
	git(t, dir, "init", "-q", "--bare", bare)
	git(t, dir, "remote", "add", "origin", bare)
	git(t, dir, "push", "-q", "origin", "main", "main~2:refs/heads/feature")
	git(t, dir, "fetch", "-q", "origin")
	git(t, dir, "tag", "v1", "main~2")
	a.refresh(false)
	tt.Frame()
	return a, tt, dir, bare
}

func TestRemoteBranchesPanel(t *testing.T) {
	a, tt, dir, _ := panelsRepo(t)
	press(tt, 0, ui.Key5)
	if a.section != secRemotes || a.remoteName != "origin" {
		t.Fatalf("section %v, remote %q", a.section, a.remoteName)
	}
	// Newest first: main, then feature.
	press(tt, 0, ui.KeyJ)
	for _, s := range []string{"2 branches on origin", "feature", "main", "Commits on origin/feature"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "remote-branches")
	// Space checks feature out as a local branch tracking it.
	press(tt, 0, ui.KeySpace)
	if got := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD@{upstream}"); got != "origin/feature" {
		t.Fatalf("upstream %q", got)
	}
	// Its commits take the keys, and copy.
	press(tt, 0, ui.KeyL)
	press(tt, ui.Shift, ui.KeyC)
	if !tt.HasText("1 commit copied from origin/feature") {
		t.Fatalf("no copy in %q", tt.Texts())
	}
	press(tt, 0, ui.KeyEscape)
	if a.pane != paneList {
		t.Fatalf("pane %v", a.pane)
	}
}

func TestTagsPanel(t *testing.T) {
	a, tt, dir, bare := panelsRepo(t)
	press(tt, 0, ui.Key6)
	if !tt.HasText("v1") || !tt.HasText("Commits on v1") {
		t.Fatalf("no v1 in %q", tt.Texts())
	}
	press(tt, 0, ui.KeyN)
	answer(t, a, tt, "New tag on HEAD", "v2", "Next")
	answer(t, a, tt, "Message for v2", "Release two", "Create Tag")
	if got := git(t, dir, "cat-file", "-t", "v2"); got != "tag" {
		t.Fatalf("v2 is a %s", got)
	}
	if !tt.HasText("Release two") {
		t.Fatalf("no v2 in %q", tt.Texts())
	}
	snapshot(t, tt, "tags")
	// The tag made is chosen: push it to the only remote.
	if got := a.selectedTag().Name; got != "v2" {
		t.Fatalf("chosen %q after making v2", got)
	}
	press(tt, ui.Shift, ui.KeyP)
	confirmWith(t, a, tt, "Push v2 to origin?", "Push")
	if got := git(t, bare, "tag"); got != "v2" {
		t.Fatalf("remote tags %q", got)
	}
	press(tt, 0, ui.KeyD)
	confirmWith(t, a, tt, "Delete tag v2?", "Delete")
	if got := git(t, dir, "tag"); got != "v1" {
		t.Fatalf("tags %q", got)
	}
	// t tags the commit chosen in History.
	press(tt, 0, ui.Key2, ui.KeyJ, ui.KeyT)
	answer(t, a, tt, "New tag on "+a.snap.Commits[1].Hash(), "v-four", "Next")
	answer(t, a, tt, "Message for v-four", "", "Create Tag")
	if got := git(t, dir, "log", "-1", "--format=%s", "v-four"); got != "four" {
		t.Fatalf("v-four is on %q", got)
	}
}

func TestReflogPanel(t *testing.T) {
	a, tt, dir := linear(t)
	git(t, dir, "reset", "-q", "--hard", "HEAD~2")
	a.refresh(false)
	tt.Frame()
	press(tt, 0, ui.Key7)
	if !tt.HasText("reset") || !tt.HasText("moving to HEAD~2") {
		t.Fatalf("no reset in %q", tt.Texts())
	}
	snapshot(t, tt, "reflog")
	// The entry before the reset is five's commit: reset back to it.
	press(tt, 0, ui.KeyJ, ui.KeyG)
	confirmWith(t, a, tt, "Reset main hard to "+a.snap.Reflog[1].ShortHash()+"?", "Reset Hard")
	if got := log(t, dir); got != "five four three two one" {
		t.Fatalf("log: %s", got)
	}
}

func TestWorktreesPanel(t *testing.T) {
	a, tt, dir := linear(t)
	var opened []string
	a.host.OpenRecent = func(path string) { opened = append(opened, path) }
	press(tt, 0, ui.Key8)
	if !tt.HasText("1 worktree") {
		t.Fatalf("no worktree in %q", tt.Texts())
	}
	press(tt, 0, ui.KeyN)
	repoName := filepath.Base(dir)
	answer(t, a, tt, "New worktree from main", "../"+repoName+"-fix", "Next")
	answer(t, a, tt, "Branch for the worktree", "fix", "Create Worktree")
	path := filepath.Join(filepath.Dir(dir), repoName+"-fix")
	t.Cleanup(func() { os.RemoveAll(path) })
	if !tt.HasText("2 worktrees") || !tt.HasText("fix") {
		t.Fatalf("no new worktree in %q", tt.Texts())
	}
	snapshot(t, tt, "worktrees")
	// Linked worktrees list after the main one: open it, then remove it,
	// with a change, which asks again.
	press(tt, 0, ui.KeyJ, ui.KeySpace)
	if len(opened) != 1 || !strings.HasSuffix(opened[0], repoName+"-fix") {
		t.Fatalf("opened %v", opened)
	}
	write(t, path, "wip.txt", "wip\n")
	press(tt, 0, ui.KeyD)
	confirmWith(t, a, tt, "Remove the worktree "+a.selectedWorktree().Name+"?", "Remove")
	confirmWith(t, a, tt, a.selectedWorktree().Name+" has changes", "Remove Anyway")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("worktree still there: %v", err)
	}
}

func TestSubmodulesPanel(t *testing.T) {
	a, tt, dir := linear(t)
	git(t, dir, "config", "--global", "protocol.file.allow", "always")
	lib := t.TempDir()
	git(t, lib, "init", "-q", "-b", "main")
	write(t, lib, "lib.txt", "lib\n")
	git(t, lib, "add", ".")
	git(t, lib, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "-m", "lib")
	press(tt, 0, ui.Key9)
	if !tt.HasText("No submodules") {
		t.Fatalf("submodules in %q", tt.Texts())
	}
	press(tt, 0, ui.KeyN)
	answer(t, a, tt, "Add a submodule", lib, "Next")
	answer(t, a, tt, "Folder for "+filepath.Base(lib), "vendor/lib", "Add Submodule")
	if !tt.HasText("vendor/lib") {
		t.Fatalf("no submodule in %q", tt.Texts())
	}
	git(t, dir, "commit", "-q", "-m", "add lib")
	git(t, dir, "submodule", "deinit", "-q", "-f", "vendor/lib")
	a.refresh(false)
	tt.Frame()
	if !tt.HasText("not initialized") {
		t.Fatalf("no state in %q", tt.Texts())
	}
	press(tt, 0, ui.KeyU)
	if _, err := os.Stat(filepath.Join(dir, "vendor/lib/lib.txt")); err != nil {
		t.Fatal(err)
	}
	snapshot(t, tt, "submodules")
}

// The sidebar goes to the new sections, and to a remote's branches.
func TestSidebarSections(t *testing.T) {
	a, tt, _, _ := panelsRepo(t)
	for _, s := range []struct {
		label string
		want  section
	}{{"Tags", secTags}, {"Reflog", secReflog}, {"Worktrees", secWorktrees}, {"Submodules", secSubmodules}, {"origin", secRemotes}} {
		if err := tt.Click(s.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.section != s.want {
			t.Fatalf("%s: section %v", s.label, a.section)
		}
	}
}
