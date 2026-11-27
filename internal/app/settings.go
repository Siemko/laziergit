package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Siemko/laziergit/internal/core"
)

// Settings are the app's, which every window follows, kept in
// settings.json in the app's config directory.
type Settings struct {
	// Theme is "system", "light" or "dark".
	Theme string `json:"theme"`

	// ShowKeyHints shows the status bar of keys; ShowTabShortcuts the
	// keys of the sidebar's sections; RestoreSession opens the last
	// session's windows at launch.
	ShowKeyHints     bool `json:"showKeyHints"`
	ShowTabShortcuts bool `json:"showTabShortcuts"`
	RestoreSession   bool `json:"restoreSession"`

	// SplitDiff shows diffs side by side, unless a window chose otherwise.
	SplitDiff   bool `json:"splitDiff"`
	DiffContext int  `json:"diffContext"`
	CommitLimit int  `json:"commitLimit"`

	// FetchMinutes fetches in the background that often, 0 never.
	FetchMinutes int      `json:"fetchMinutes"`
	SignOff      bool     `json:"signOff"`
	ConfirmPush  bool     `json:"confirmPush"`
	MainBranches []string `json:"mainBranches"`
}

// DefaultSettings are the settings of a new install, as the app was.
func DefaultSettings() Settings {
	c := core.DefaultConfig()
	return Settings{
		Theme:            "system",
		ShowKeyHints:     true,
		ShowTabShortcuts: true,
		RestoreSession:   true,
		DiffContext:      c.DiffContext,
		CommitLimit:      c.CommitLimit,
		MainBranches:     c.MainBranches,
	}
}

func settingsFile(dir string) string { return filepath.Join(dir, "settings.json") }

// LoadSettings reads the settings in dir, the defaults for those it lacks,
// or all of them when there are none yet.
func LoadSettings(dir string) (Settings, error) {
	s := DefaultSettings()
	if dir == "" {
		return s, nil
	}
	data, err := os.ReadFile(settingsFile(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettings(), err
	}
	return s.normalized(), nil
}

// SaveSettings writes the settings in dir, atomically.
func SaveSettings(dir string, s Settings) error {
	data, err := json.MarshalIndent(s.normalized(), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), settingsFile(dir))
}

// normalized holds the settings to what the app can use.
func (s Settings) normalized() Settings {
	switch s.Theme {
	case "light", "dark":
	default:
		s.Theme = "system"
	}
	c := s.Core()
	s.DiffContext, s.CommitLimit, s.MainBranches = c.DiffContext, c.CommitLimit, c.MainBranches
	s.FetchMinutes = max(0, min(s.FetchMinutes, 24*60))
	return s
}

// Core is what of the settings git's commands follow.
func (s Settings) Core() core.Config {
	return core.Config{
		DiffContext:  s.DiffContext,
		CommitLimit:  s.CommitLimit,
		SignOff:      s.SignOff,
		MainBranches: s.MainBranches,
	}.Normalized()
}

// settings is the window's settings: the app's, or the defaults for a
// window without them, as in tests.
func (a *App) settings() Settings {
	if a.host.Settings != nil {
		return *a.host.Settings
	}
	return DefaultSettings()
}

// SettingsChanged follows settings changed: the repository's commands and,
// for the diffs and the log, a load again.
func (a *App) SettingsChanged() {
	s := a.settings()
	if s.SplitDiff != a.splitDefault {
		a.splitDefault = s.SplitDiff
		a.unified = 0
		if s.SplitDiff {
			a.unified = 1
		}
	}
	a.configure(func() {
		a.detailKey, a.diffKey = "", "" // diffs read again
		a.refresh(true)
	})
}

// configure gives the repository the settings, off the UI thread, then
// runs then, on the UI thread, as a load the settings change.
func (a *App) configure(then func()) {
	repo := a.repo
	if repo == nil {
		return
	}
	cfg := a.settings().Core()
	a.host.Go(func() {
		err := repo.Configure(cfg)
		a.host.Post(func() {
			if repo != a.repo {
				return
			}
			if err != nil {
				a.addLog("settings: "+err.Error(), false)
			}
			then()
		})
	})
}

// AutoFetch fetches in the background, as the settings ask every few
// minutes: quietly, and not while the window has git busy or a dialog up.
func (a *App) AutoFetch() {
	if a.repo == nil || a.snap == nil || len(a.snap.Remotes) == 0 || a.busy > 0 || a.loading || a.dialog != nil {
		return
	}
	repo := a.repo
	a.host.Go(func() {
		err := repo.FetchBackground()
		a.host.Post(func() {
			if repo != a.repo {
				return
			}
			if err != nil {
				a.addLog("background fetch: "+err.Error(), false)
				return
			}
			if a.busy == 0 {
				a.refresh(true)
			}
		})
	})
}

// SettingsPane is the Settings window: the settings s, which changed is
// told of after each change, on the UI thread.
type SettingsPane struct {
	S       *Settings
	Changed func()
	tab     int
	// mains is the main branches as typed, kept while they are edited.
	mains    string
	mainsSet bool
}

// View builds the Settings window.
func (p *SettingsPane) View(c *ui.Context) {
	tt := c.Theme()
	pal := paletteFor(tt)
	s := p.S
	changed := func() {
		*s = s.normalized()
		if p.Changed != nil {
			p.Changed()
		}
	}
	if !p.mainsSet {
		p.mains, p.mainsSet = strings.Join(s.MainBranches, ", "), true
	}
	ui.Column(c).Fill().Background(tt.Background).Children(func() {
		// The title bar, with the tabs, as macOS's settings windows have.
		ui.Row(c).Height(titleBarHeight).Shrink(0).Center().DragWindow().
			BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Background(pal.header).Children(func() {
			ui.Segmented(c.Key("tabs"), &p.tab, "General", "Diffs & History", "Git").Label("Settings section")
		})
		ui.Scroll(c.Key("settings")).Grow(1).Children(func() {
			ui.Column(c).Padding(24, 32, 28, 32).Gap(18).Children(func() {
				switch p.tab {
				case 0:
					p.general(c, s, changed)
				case 1:
					p.diffs(c, s, changed)
				case 2:
					p.git(c, s, changed)
				}
			})
		})
	})
}

func (p *SettingsPane) general(c *ui.Context, s *Settings, changed func()) {
	ui.Form(c, func() {
		ui.Fieldset(c, "Appearance", func() {
			themes := []string{"system", "light", "dark"}
			i := max(0, indexOf(themes, s.Theme))
			ui.Field(c, "Theme", func() {
				if ui.Segmented(c.Key("theme"), &i, "System", "Light", "Dark").AlignSelf(ui.Start).Changed() {
					s.Theme = themes[i]
					changed()
				}
			})
		})
		ui.Fieldset(c, "Interface", func() {
			ui.Field(c, "Key hints", func() {
				if ui.Switch(c.Key("hints"), &s.ShowKeyHints).Changed() {
					changed()
				}
			}).Description("The keys of the pane with the focus, in the status bar.")
			ui.Field(c, "Tab shortcuts", func() {
				if ui.Switch(c.Key("tabKeys"), &s.ShowTabShortcuts).Changed() {
					changed()
				}
			}).Description("The key of each section (1–9), in the sidebar.")
		})
		ui.Fieldset(c, "Launch", func() {
			ui.Field(c, "Reopen windows", func() {
				if ui.Switch(c.Key("restore"), &s.RestoreSession).Changed() {
					changed()
				}
			}).Description("The repositories open when the app quit open again.")
		})
	})
}

func (p *SettingsPane) diffs(c *ui.Context, s *Settings, changed func()) {
	ui.Form(c, func() {
		ui.Fieldset(c, "Diffs", func() {
			layout := 0
			if s.SplitDiff {
				layout = 1
			}
			ui.Field(c, "Layout", func() {
				if ui.Segmented(c.Key("layout"), &layout, "Unified", "Split").AlignSelf(ui.Start).Changed() {
					s.SplitDiff = layout == 1
					changed()
				}
			}).Description("How diffs show at first; ⌥S switches in a window.")
			ctx := float64(s.DiffContext)
			ui.Field(c, "Context lines", func() {
				if ui.NumberInput(c.Key("context"), &ctx, 1, 20, 1).Width(130).Changed() {
					s.DiffContext = int(ctx)
					changed()
				}
			}).Description("The unchanged lines shown around each change.")
		})
		ui.Fieldset(c, "History", func() {
			limit := float64(s.CommitLimit)
			ui.Field(c, "Commits loaded", func() {
				if ui.NumberInput(c.Key("limit"), &limit, 50, 5000, 50).Width(130).Changed() {
					s.CommitLimit = int(limit)
					changed()
				}
			}).Description("The latest commits History shows; more take longer to load.")
		})
	})
}

func (p *SettingsPane) git(c *ui.Context, s *Settings, changed func()) {
	ui.Form(c, func() {
		ui.Fieldset(c, "Remotes", func() {
			fetch := s.FetchMinutes > 0
			ui.Field(c, "Fetch in background", func() {
				if ui.Switch(c.Key("fetch"), &fetch).Changed() {
					s.FetchMinutes = 0
					if fetch {
						s.FetchMinutes = 5
					}
					changed()
				}
			}).Description("Keeps the remote branches and ahead/behind up to date. Remotes that ask for a password are left alone.")
			if fetch {
				minutes := float64(s.FetchMinutes)
				ui.Field(c, "Every", func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if ui.NumberInput(c.Key("minutes"), &minutes, 1, 120, 1).Width(130).Changed() {
							s.FetchMinutes = int(minutes)
							changed()
						}
						ui.Text(c, "minutes").TextColor(paletteFor(c.Theme()).muted)
					})
				})
			}
			ui.Field(c, "Ask before pushing", func() {
				if ui.Switch(c.Key("confirmPush"), &s.ConfirmPush).Changed() {
					changed()
				}
			})
		})
		ui.Fieldset(c, "Commits", func() {
			ui.Field(c, "Sign off", func() {
				if ui.Switch(c.Key("signOff"), &s.SignOff).Changed() {
					changed()
				}
			}).Description("Adds a Signed-off-by line, as git commit --signoff.")
			ui.Field(c, "Main branches", func() {
				in := ui.TextInput(c.Key("mains"), &p.mains).Placeholder("master, main").Width(220)
				if in.Changed() {
					s.MainBranches = splitList(p.mains)
					changed()
				}
			}).Description("Commits on them show as merged, and History's pushed state counts from them.")
		})
	})
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// splitList is a list typed with commas or spaces between its items.
func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
}
