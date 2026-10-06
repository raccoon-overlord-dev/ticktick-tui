package ui

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
)

// The New task panel (n, or ": New task") edits a draft task with the details pane's
// fields and actions; nothing is sent until Create. While it is open the detail actions
// (see target) work on the draft, and update only changes it locally.

func (a *App) openDraft() {
	add := a.quickAdd("") // the current list and, in Today / Tomorrow, that day
	t := &api.Task{ID: "draft", ProjectID: strings.TrimPrefix(add.List, "p:")}
	setDueFields(t, add.Due)
	a.draft, a.draftInit = t, store.Clone(*t)
	a.mainDF, a.mainOff = a.df, a.offDetail
	a.df, a.offDetail, a.offDraft, a.cmd = 0, 0, 0, nil
}

func (a *App) closeDraft() {
	a.draft, a.draftAsk, a.edit = nil, false, ""
	a.df, a.offDetail = a.mainDF, a.mainOff
}

func draftKeys(t *api.Task) []string { return append(detailKeys(t), "create") }

func (a *App) draftKey(k tea.KeyPressMsg) tea.Cmd {
	t := a.draft
	keys := draftKeys(t)
	a.df = min(a.df, len(keys)-1)
	key := k.String()
	if a.cfg.Keys.Keymap != "arrows" {
		if alias, ok := map[string]string{"j": "down", "k": "up", "g": "home", "G": "end"}[key]; ok {
			key = alias
		}
	}
	field := keys[a.df]
	item := field != "additem" && field != "create" && strings.HasPrefix(field, "c")
	switch key {
	case "esc":
		if reflect.DeepEqual(*t, a.draftInit) {
			a.closeDraft()
			return nil
		}
		a.draftAsk = true
		a.flash, a.flashRole, a.flashUntil = "Discard the new task? y / N", "warn", time.Now().Add(time.Hour)
	case "ctrl+s":
		return a.createDraft()
	case "down", "tab":
		a.df = step(a.df, 1, len(keys))
	case "up", "shift+tab":
		a.df = step(a.df, -1, len(keys))
	case "home":
		a.df = 0
	case "end":
		a.df = len(keys) - 1
	case "enter":
		if field == "create" {
			return a.createDraft()
		}
		return a.activate()
	case "i", "e", "ctrl+a", "super+a":
		if field == "title" || field == "due" || field == "repeat" || field == "tags" || field == "notes" || field == "additem" || item {
			a.startEdit(field)
			if strings.HasSuffix(key, "+a") {
				a.in.selectAll()
			}
		}
	case "x", "space":
		if item {
			var i int
			fmt.Sscanf(field, "c%d", &i)
			return a.toggleCheck(t, i)
		}
	case "p":
		return a.cyclePrio(t)
	case "d":
		a.duePicker(t)
	case "m":
		a.movePicker(t)
	case "c":
		a.startEdit("additem")
	}
	return nil
}

func (a *App) discardKey(k tea.KeyPressMsg) tea.Cmd {
	a.draftAsk, a.flash = false, ""
	if k.String() == "y" {
		a.closeDraft()
	}
	return nil
}

// createDraft sends the draft as one create. A task with checklist items is a CHECKLIST,
// which keeps its notes in desc.
func (a *App) createDraft() tea.Cmd {
	t := *a.draft
	if strings.TrimSpace(t.Title) == "" {
		a.df = 0
		a.setFlash("type a title first")
		return nil
	}
	if len(t.Items) > 0 {
		t.Kind, t.Desc, t.Content = "CHECKLIST", t.Content, ""
	}
	a.closeDraft()
	return a.addTask(t)
}

// viewDraft renders the New task panel and returns it with its size.
func (a *App) viewDraft() (string, int, int) {
	p := a.overlayPen()
	w := min(64, a.w-4)
	keys := draftKeys(a.draft)
	lines, sel := a.detailLines(p, a.draft, w, true)
	cp := p
	if keys[min(a.df, len(keys)-1)] == "create" {
		cp, sel = p.on(a.selBg(true)), len(lines)+1
	}
	lines = append(lines, "", cp.line(w, cp.s("ok").Bold(true).Render("[ Create ]"), cp.s("dim").Render("ctrl+s")))
	footer := []string{p.s("line").Render(strings.Repeat("─", w-2)),
		p.line(w, p.s("dim").Render("↑↓ field · ⏎ edit / pick · ctrl+s create · esc cancel"), "")}
	body := append(window(lines, sel, max(a.h-3-len(footer), 3), &a.offDraft), footer...)
	h := len(body) + 2
	return a.frame(p, frameOpts{w: w, h: h, title: "New task", modal: true, body: body}), w, h
}
