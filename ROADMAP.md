# Roadmap

Ideas planned after v1. Not implemented yet.

## Known bugs

- **Rows spill into the next column in Ghostty**: some task rows (seen with titles containing `⚠️`) draw wider than the pane, push into the next column, and the rows below shift (list names appear twice while scrolling). Fixed in Terminal.app, still there in Ghostty (macOS and Omarchy) as of v0.2.0. Alacritty on Omarchy is fine with v0.2.0, so this looks Ghostty-specific; parked as a known issue.
  - Tried: Bubble Tea's renderer counts `⚠️`/`♻️`/`👨🏻‍💻` with wcwidth unless the terminal confirms mode 2027, while lipgloss uses grapheme widths. v0.1.2 queried mode 2027 (Ghostty answers `2027;1$y`, set); v0.1.3/v0.2.0 rewrite those clusters on every frame (`wcSafe` in `internal/ui/render.go`) so both counts agree. Terminal.app is fixed; Ghostty is not, so the cause there is something else.
  - Next ideas: record the raw output in Ghostty (`script -q out.txt ./ttui`) and find the first row that goes wrong; check other glyphs on those rows (Nerd Font icons are private-use code points that Ghostty may draw 2 wide; `✓`); try Ghostty's `grapheme-width-method = legacy`; reproduce with a minimal Bubble Tea program and report upstream if it's the renderer.
- **Duplicated first task on `↓`**: with the first task selected, pressing `↓` draws that task twice. It's only visual; the app/web UI shows one task. Seen on Omarchy + Ghostty, not on macOS + Ghostty, so it may not be a ttui issue.

## Mouse support

- Clicking a pane focuses it; clicking a list, task, checklist item or the notes block selects it, and clicking the selected item again opens it (same as ⏎).
- Toggle in Settings (`Mouse: on / off`) and `config.toml` (`[appearance] mouse = true`).
- Implementation: `tea.View.MouseMode = tea.MouseModeCellMotion` when enabled, map click coordinates to rows using the pane layout.
- Wheel scrolling (tried and pulled for now): map `tea.MouseWheelMsg` to ↑↓. Caveats found: mouse mode makes text selection need `shift`, and with mouse off most terminals still turn the wheel into ↑↓ in the alternate screen ("alternate scroll", `CSI ?1007 l` turns it off, but restoring it on exit means guessing its old state).
- Clickable `https://` links inside notes (may come for free via OSC 8 hyperlinks, without mouse mode).

## Tasks

- Edit `due_menu` from Settings (today it's in `config.toml` only).
- Manual task order, like the GUI: overrides the list's sort for that task.
- Repeat rules in quick add (e.g. `every week`); today a repeat is set from the details pane.
- Completed history beyond the last 7 days.

## Lists

- Create folders and lists.
- Kanban boards: display and interact with lists in board view.

## Editing and navigation

- Cursor blink while editing notes.
- Better insert mode for task details.
- (To be confirmed) pane shortcuts: `L` Lists, `C` current list, `D` Details.

## Distribution

- Check for new releases on startup and offer to update (toggle in Settings).

- CI release workflow (GitHub Actions + GoReleaser). Needs a token with the `workflow` scope; releases are built locally with `make dist` for now.

- Homebrew tap, AUR package, .deb/.rpm (v1 ships `install.sh` only).
