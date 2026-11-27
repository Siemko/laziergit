package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

// The app updates itself from the releases on GitHub (mygo.json's
// updates), in the update window of MyGo's updater: it checks in the
// background once a day and when asked. Installed with Homebrew, it
// leaves updating to Homebrew, which would lose track of an app that
// replaced itself in its Cellar.

// fromHomebrew reports whether the app runs from Homebrew's Cellar, as
// the formula installs it; through a link in /Applications too.
func fromHomebrew() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return inCellar(exe)
}

// inCellar reports whether a path is in a Homebrew Cellar, as
// /opt/homebrew/Cellar/laziergit/0.6.0/laziergit.app is.
func inCellar(path string) bool { return strings.Contains(path, "/Cellar/") }

// useUpdater starts the background checks, but for an app Homebrew
// updates. It must run before App.Run.
func useUpdater() {
	if !fromHomebrew() {
		mygo.Use(native.Plugin)
	}
}

// updatesMenuItem is "Check for Updates…": the updater's, or, installed
// with Homebrew, one that says how Homebrew updates it.
func updatesMenuItem() *mygo.MenuItem {
	if !fromHomebrew() {
		return updater.MenuItem()
	}
	return &mygo.MenuItem{Label: "Check for Updates…", Click: func(*mygo.MenuItem, *mygo.Window) {
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{
			Type:    mygo.MessageInfo,
			Message: "laziergit was installed with Homebrew",
			Detail:  "Homebrew updates it: run brew upgrade laziergit in a terminal, then open laziergit again.",
		})
	}}
}
