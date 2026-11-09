package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// A confirmation of what can't be undone doesn't take Enter, or a stray
// key after d, for a yes: Cancel has the focus.
func TestEnterDoesntConfirmDiscard(t *testing.T) {
	a, tt, dir := newTester(t)
	before := git(t, dir, "status", "--porcelain")
	tt.Key(0, ui.KeyD)
	tt.Frame()
	if a.dialog == nil || a.dialog.button != "Discard" {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.dialog != nil {
		t.Fatal("Enter left the confirmation open")
	}
	if got := git(t, dir, "status", "--porcelain"); got != before {
		t.Fatalf("Enter discarded: %q, was %q", got, before)
	}
}

// A file whose diff is too large to read and draw says so, in place of
// the diff.
func TestLargeDiffSaysSo(t *testing.T) {
	dir := fixture(t)
	write(t, dir, "big.txt", strings.Repeat("a line of a large file\n", 9<<20/20))
	a, tt, _ := testerFor(t, dir)
	for i := 0; i < 10; i++ {
		if f := a.selectedFile(); f != nil && f.Path == "big.txt" {
			break
		}
		tt.Key(0, ui.KeyJ)
		tt.Frame()
	}
	tt.Frame()
	if !tt.HasText("Too large to show here") {
		t.Fatalf("no word of the size: %q", tt.Texts())
	}
}

// The first push of a branch, with several remotes and no word from git's
// config on which, asks for the remote rather than take the first.
func TestFirstPushAsksForRemote(t *testing.T) {
	dir := fixture(t)
	origin, public := t.TempDir(), t.TempDir()
	git(t, origin, "init", "-q", "--bare")
	git(t, public, "init", "-q", "--bare")
	git(t, dir, "remote", "add", "origin", origin)
	git(t, dir, "remote", "add", "public", public)
	a, tt, _ := testerFor(t, dir)

	tt.Key(ui.Shift, ui.KeyP)
	tt.Frame()
	if a.dialog == nil || a.dialog.value != "origin" || !tt.HasText("public") {
		t.Fatalf("no remote asked for: %q", tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("public ")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if got := git(t, public, "for-each-ref", "--format=%(refname)"); got != "refs/heads/main" {
		t.Fatalf("public has %q", got)
	}
	if got := git(t, origin, "for-each-ref"); got != "" {
		t.Fatalf("origin has %q", got)
	}
}

// A push asked for while git waits on a password leaves its prompt as it
// is: the push's question doesn't take its place, nor its words.
func TestPushLeavesCredentialPromptAlone(t *testing.T) {
	dir := fixture(t)
	for _, name := range []string{"origin", "public"} {
		bare := t.TempDir()
		git(t, bare, "init", "-q", "--bare")
		git(t, dir, "remote", "add", name, bare)
	}
	a, tt, _ := testerFor(t, dir)
	answer := a.opts.AskCredential(0)
	prompt := *a.dialog
	a.push(false)
	tt.Frame()
	if a.dialog.kind != dialogCredential || a.dialog.title != prompt.title || a.dialog.message != prompt.message {
		t.Fatalf("credential prompt now %q: %q", a.dialog.title, a.dialog.message)
	}
	tt.Frame()
	if !tt.HasText("Answer git's prompt first") {
		t.Fatalf("the push went unsaid: %q", tt.Texts())
	}
	a.dialog.onCancel()
	<-answer
}
