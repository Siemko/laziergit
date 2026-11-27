package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsFile(t *testing.T) {
	dir := t.TempDir()
	if s, err := LoadSettings(dir); err != nil || s.Theme != "system" || !s.ShowKeyHints || s.DiffContext != 3 {
		t.Fatalf("defaults %+v, %v", s, err)
	}
	s := DefaultSettings()
	s.Theme, s.FetchMinutes, s.MainBranches = "dark", 10, []string{"trunk", "-x"}
	if err := SaveSettings(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(dir)
	if err != nil || got.Theme != "dark" || got.FetchMinutes != 10 || !slices.Equal(got.MainBranches, []string{"trunk"}) {
		t.Fatalf("loaded %+v, %v", got, err)
	}
	if st, _ := os.Stat(filepath.Join(dir, "settings.json")); st.Mode().Perm() != 0o600 {
		t.Fatalf("settings.json is %v", st.Mode().Perm())
	}
	// A file of an older version keeps the defaults of what it lacks.
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":"light"}`), 0o600)
	if got, _ := LoadSettings(dir); got.Theme != "light" || !got.ShowTabShortcuts || got.CommitLimit != 300 {
		t.Fatalf("old file %+v", got)
	}
}

// withSettings opens linear's repository in a window with settings s.
func withSettings(t *testing.T, s *Settings) (*App, *ui.Tester, string) {
	a, tt, dir := linear(t)
	a.host.Settings = s
	a.SettingsChanged()
	tt.Frame()
	return a, tt, dir
}

func TestTabShortcutsAndHints(t *testing.T) {
	s := DefaultSettings()
	a, tt, _ := withSettings(t, &s)
	for _, label := range []string{"Shortcut 1", "Shortcut 9"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("no %q", label)
		}
	}
	if !tt.HasText("Commands") || !tt.HasText("Reword Commit") {
		t.Fatalf("no hints in %q", tt.Texts())
	}
	snapshot(t, tt, "tab-shortcuts")
	s.ShowTabShortcuts, s.ShowKeyHints = false, false
	a.SettingsChanged()
	tt.Frame()
	if _, ok := tt.Find("Shortcut 1"); ok {
		t.Fatal("shortcuts still shown")
	}
	if tt.HasText("Reword Commit") {
		t.Fatal("hints still shown")
	}
}

func TestSettingsApply(t *testing.T) {
	s := DefaultSettings()
	s.CommitLimit, s.SplitDiff = 2, true
	a, tt, _ := withSettings(t, &s)
	if len(a.snap.Commits) != 2 || !tt.HasText("Latest 2 commits") {
		t.Fatalf("%d commits, in %q", len(a.snap.Commits), tt.Texts())
	}
	if a.unified != 1 {
		t.Fatal("diffs not split")
	}
}

func TestConfirmPush(t *testing.T) {
	s := DefaultSettings()
	s.ConfirmPush = true
	a, tt, dir, bare := panelsRepoWith(t, &s)
	git(t, dir, "branch", "-u", "origin/main")
	write(t, dir, "six.txt", "six\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "six")
	a.refresh(false)
	tt.Frame()
	press(tt, ui.Shift, ui.KeyP)
	confirmWith(t, a, tt, "Push main to origin/main?", "Push")
	if got := git(t, bare, "log", "-1", "--format=%s", "main"); got != "six" {
		t.Fatalf("remote main at %q", got)
	}
}

// panelsRepoWith is panelsRepo, in a window with settings s.
func panelsRepoWith(t *testing.T, s *Settings) (*App, *ui.Tester, string, string) {
	a, tt, dir, bare := panelsRepo(t)
	a.host.Settings = s
	a.SettingsChanged()
	tt.Frame()
	return a, tt, dir, bare
}

func TestSettingsWindow(t *testing.T) {
	s := DefaultSettings()
	changes := 0
	p := &SettingsPane{S: &s, Changed: func() { changes++ }}
	tt := ui.NewTester(p.View, 580, 460)
	tt.Frame()
	for _, text := range []string{"General", "Theme", "Key hints", "Tab shortcuts", "Reopen windows"} {
		if !tt.HasText(text) {
			t.Fatalf("no %q in %q", text, tt.Texts())
		}
	}
	snapshot(t, tt, "settings-general")
	if err := tt.Click("Dark"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.Theme != "dark" || changes != 1 {
		t.Fatalf("theme %q after %d changes", s.Theme, changes)
	}
	if err := tt.Click("Tab shortcuts"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.ShowTabShortcuts {
		t.Fatal("tab shortcuts still on")
	}
	if err := tt.Click("Diffs & History"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Split"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !s.SplitDiff || !tt.HasText("Commits loaded") {
		t.Fatalf("split %v, in %q", s.SplitDiff, tt.Texts())
	}
	snapshot(t, tt, "settings-diffs")
	if err := tt.Click("Git"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Fetch in background"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.FetchMinutes != 5 || !tt.HasText("minutes") {
		t.Fatalf("fetch every %d, in %q", s.FetchMinutes, tt.Texts())
	}
	snapshot(t, tt, "settings-git")
}
