package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"ttui/internal/update"
)

type (
	updateMsg     string // newer release tag
	updateDoneMsg struct {
		exe string
		err error
	}
)

// checkUpdate looks for a newer release in the background; failures stay silent.
func (a *App) checkUpdate() tea.Cmd {
	if !a.cfg.Account.UpdateCheck || a.demo || !update.Newer(a.version, "0") { // last: dev builds have no version number
		return nil
	}
	current := a.version
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if tag, err := update.Check(ctx); err == nil && update.Newer(tag, current) {
			return updateMsg(tag)
		}
		return nil
	}
}

// askUpdate is U: confirm before replacing the binary.
func (a *App) askUpdate() {
	switch {
	case a.updating:
	case a.newVersion == "":
		a.setFlash("no update available · ttui " + a.version)
	default:
		a.confirmUpdate = true
		a.flash, a.flashRole, a.flashUntil = "Update to "+a.newVersion+" and restart? y / n", "warn", time.Now().Add(time.Hour)
	}
}

func (a *App) confirmKey(k tea.KeyPressMsg) tea.Cmd {
	a.confirmUpdate, a.flash = false, ""
	if k.String() != "y" {
		return nil
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		a.flashError("✗ update: " + err.Error())
		return nil
	}
	a.updating = true
	a.flash, a.flashRole, a.flashUntil = "downloading "+a.newVersion+"…", "info", time.Now().Add(time.Hour)
	tag := a.newVersion
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		return updateDoneMsg{exe, update.Apply(ctx, tag, exe)}
	}
}

func (a *App) onUpdated(m updateDoneMsg) tea.Cmd {
	a.updating = false
	switch {
	case errors.Is(m.err, update.ErrNotWritable):
		a.flashError("✗ no write access to the ttui binary · install command copied: quit and paste it")
		a.flashUntil = time.Now().Add(15 * time.Second)
		return tea.SetClipboard(update.Command)
	case m.err != nil:
		a.flashError("✗ update failed: " + m.err.Error())
		a.flashUntil = time.Now().Add(15 * time.Second)
		return nil
	}
	a.restart = m.exe
	return a.restartIfIdle()
}

// restartIfIdle quits (and run execs the new binary) once no change is waiting to be sent.
func (a *App) restartIfIdle() tea.Cmd {
	if a.restart == "" {
		return nil
	}
	if a.busy || len(a.queue) > 0 {
		a.flash, a.flashRole, a.flashUntil = "updated · restarting once your changes are saved…", "info", time.Now().Add(time.Hour)
		return nil
	}
	return tea.Quit
}
