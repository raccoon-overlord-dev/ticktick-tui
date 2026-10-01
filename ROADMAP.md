# Roadmap

Ideas planned after v1. Not implemented yet.

## Known bugs

- **Duplicated first task on `↓`**: with the first task selected, pressing `↓` draws that task twice. It's only visual; the app/web UI shows one task. Seen on Omarchy + Ghostty, not on macOS + Ghostty, so it may not be a ttui issue.

## Mouse support

- Clicking a pane focuses it; clicking a list, task, checklist item or the notes block selects it, and clicking the selected item again opens it (same as ⏎).
- Toggle in Settings (`Mouse: on / off`) and `config.toml` (`[appearance] mouse = true`).
- Implementation: `tea.View.MouseMode = tea.MouseModeCellMotion` when enabled, map click coordinates to rows using the pane layout.
- Clickable `https://` links inside notes (may come for free via OSC 8 hyperlinks, without mouse mode).

## Tasks

- Edit `due_menu` from Settings (today it's in `config.toml` only).
- Manual task order, like the GUI: overrides the list's sort for that task.
- Sorting revamp matching the web UI: Group by + Sort by + Order.
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
