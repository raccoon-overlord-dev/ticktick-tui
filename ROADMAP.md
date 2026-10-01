# Roadmap

Ideas planned after v1. Not implemented yet.

## Mouse support

- Clicking a pane focuses it; clicking a list, task, checklist item or the notes block selects it, and clicking the selected item again opens it (same as ⏎).
- Toggle in Settings (`Mouse: on / off`) and `config.toml` (`[appearance] mouse = true`).
- Implementation: `tea.View.MouseMode = tea.MouseModeCellMotion` when enabled, map click coordinates to rows using the pane layout.

## Tasks

- Move a task to another list (API: `POST /task/move`; the details "List" field currently only shows the list).
- Repeat rules in quick add (e.g. `every week`); today a repeat is set from the details pane.
- Completed history beyond the last 7 days.

## Distribution

- CI release workflow (GitHub Actions + GoReleaser). Needs a token with the `workflow` scope; releases are built locally with `make dist` for now.

- Homebrew tap, AUR package, .deb/.rpm (v1 ships `install.sh` only).
