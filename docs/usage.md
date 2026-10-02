# ttui usage

Everything beyond the [README](../README.md): install options, sign-in, keys, quick add, configuration, themes, limits.

## Install

macOS and Linux (Arch/Omarchy, Debian/Ubuntu/Pop!_OS, Fedora). On Windows, use WSL.

```sh
curl -fsSL https://raw.githubusercontent.com/raccoon-overlord-dev/ticktick-tui/main/install.sh | sh
```

The script downloads the latest release for your OS and CPU, checks its SHA-256 against `checksums.txt`,
and installs `ttui` to `~/.local/bin` (no sudo). If that folder isn't on your `PATH`, it prints the line to add.
Run it again to upgrade, or press `U` in ttui when the status bar shows a new version: ttui checks once a day (Settings → Check for updates, `update_check` in `config.toml`), verifies the download against the release checksums, replaces itself and restarts. If it can't write its own binary (e.g. installed in a root-owned folder), it copies this install command to the clipboard instead.

| Variable | Effect |
|---|---|
| `TTUI_INSTALL_DIR` | Install somewhere else (default `~/.local/bin`) |
| `TTUI_FROM_SOURCE=/path/to/checkout` | Build a local checkout with Go instead of downloading |

Uninstall (asks before deleting your settings and sign-in):

```sh
curl -fsSL https://raw.githubusercontent.com/raccoon-overlord-dev/ticktick-tui/main/install.sh | sh -s -- --uninstall
```

From source, with Go installed: `make install` (or `go install ./cmd/ttui`).

## Sign in

Run `ttui`. The first screen offers two ways:

- **Browser (OAuth)**: ttui opens TickTick's sign-in page and waits on `localhost:8421`. Keys: `o` reopen the browser, `c` copy the URL.
  Over SSH the browser runs on your own computer, so forward the port first: `ssh -L 8421:127.0.0.1:8421 <host>`.
- **API token** (`tab`): paste the token from TickTick web → Settings → Account → API Token.

The token is stored in `~/.config/ttui/auth.toml` (mode `0600`). Sign out from Settings or the command bar.

## Layout

Three panes (lists, tasks, details) at 120 columns or more, two (tasks, details) from 80, one below that,
where details open as a bottom sheet. Force a layout in Settings → Columns.

## Keys

Vim keys and arrows both work (Settings → Keymap can switch vim keys off).

| Keys | Action |
|---|---|
| `j` `k` / `↓` `↑` | Move. In lists, moving selects the list |
| `g` / `G` | Top / bottom |
| `pgup` / `pgdn` | One page up / down (lists, tasks, long notes, this panel, the notes editor). |
| `h` `l` / `←` `→` | Previous / next pane; `l` opens a list, a folder or a task |
| `1` `2` `3`, `tab` | Focus lists / tasks / details, cycle panes |
| `⏎` | Open; on a detail field: edit it, pick a due date or list, cycle Repeat / Priority, or tick a checklist item |
| `x` / `space` | Complete or reopen the task (right after completing, `x` undoes). In lists, `space` (or `⏎`, `l`) opens or closes a folder; folders start closed and ttui remembers the open ones |
| `p` | Cycle priority High → Medium → Low → None |
| `d` | Due date menu: the dates in `due_menu`, No date, or Custom (type one). A date without a time keeps the task's time |
| `m` | Move the task to another list |
| `c` | Add checklist items: `⏎` adds one and opens the next, `esc` finishes |
| `i` / `e` | Edit the title (tasks) or the field under the cursor (details). `esc` or `⏎` saves. On the notes `↑` `↓` scroll them when they are long; while editing notes `⏎` is a newline, `↑` `↓` move between lines and `esc` / `ctrl+s` saves (`ctrl+⏎` also saves where the terminal passes it through; Omarchy uses it for fullscreen) |
| `a` / `n` | Quick add |
| `/`, `ctrl+k` | Search |
| `:` | Commands |
| `@` / `#` | Jump to a list / tag |
| `t` | Toggle due-date labels |
| `s` | Group / sort this list: Group by, Sort by, Order (like the web app's sort menu); saved per list |
| `ctrl+r` | Sync now |
| `U` | Update ttui when the status bar shows `↑ vX.Y.Z` (asks first, then restarts on the new version) |
| `,` | Settings |
| `?` | All keys |
| `esc` / `q` | Back / quit |

### Command bar

The first character picks the mode: nothing = search, `>` commands, `+` quick add, `@` lists, `#` tags.

Quick add understands `call mom tomorrow 17:00 !high #errand @home`:

- priority `!high` `!med` `!low` (or `!h` `!m` `!l`, `!3` `!2` `!1`)
- day `today`, `tomorrow`/`tmr`, weekday names (`fri` = next Friday), `+3d` (in 3 days), `2026-10-08`; time `17:00` or `5pm` (a time alone means today)
- `#tag`, `@list` (prefix of the list name)

Without a list, tasks go to the current list or Inbox. Adding from Today or Tomorrow defaults the day.

To make a task repeat, give it a due date, then press `⏎` on **Repeat** in the details pane
(never → Daily → Weekdays → Weekly → Monthly).

## Configuration

`~/.config/ttui/config.toml` (or `$XDG_CONFIG_HOME/ttui/config.toml`; `--config <file>` to override) is created
with commented defaults on first run. Every option is also in Settings (`,`), where changes apply at once and are saved.

| Section | Key | Values |
|---|---|---|
| `appearance` | `theme` | `terminal` · `colorful` · `lotr` · a custom theme name |
| | `background` | `solid` · `transparent` · `blur` (the last two let your terminal's opacity and blur show through) |
| | `priority_headers` | `rule` · `label` · `tab` |
| | `focused_panel` | `border` · `title` |
| | `nerd_font_icons` | `true` · `false` (needs a [Nerd Font](https://www.nerdfonts.com); `false` uses ASCII) |
| `layout` | `columns` | `auto` · `3` · `2` · `1` |
| | `show_completed` | `true` · `false` |
| | `open_folders` | folders shown open in Lists, written by ttui when you open or close one (folders start closed) |
| `tasks` | `due_label`, `week_start` | `true`/`false`; `mon`·`sun` |
| | `group_by` | `list` · `date` · `created` · `tag` · `priority` · `none` (default for all lists) |
| | `sort_by` | `date` · `created` · `modified` · `title` · `tag` · `priority` (default for all lists) |
| | `order` | `oldest` · `newest` (default for all lists) |
| | `list_sort` | per-list overrides written by `s`; "Use default" in the `s` menu removes one |
| | `due_menu` | dates offered by `d`, in quick-add syntax: `["today", "tomorrow", "+2d", "mon", "+7d"]` (config file only) |
| `keys` | `keymap` | `vim+arrows` · `arrows` |
| `account` | `sync_every` | `1m` · `5m` · `15m` · `manual` |
| | `update_check` | `true` · `false`: look for a new release once a day (one request to GitHub) |

### Themes

`terminal` (default) uses your terminal's 16 ANSI colors, so it follows your terminal theme. `colorful` and `lotr` are built in.
For a custom theme, copy one of [`internal/theme/themes/*.toml`](../internal/theme/themes) to
`~/.config/ttui/themes/<name>.toml`, edit the colors (`#rrggbb`, an ANSI index `0`–`255`, or `""` for the terminal default)
and set `theme = "<name>"`.

## Known limitations

These come from TickTick's public API (details in [`docs/api-notes.md`](api-notes.md)):

- No Trash and no attachments: the API doesn't expose them.
- Smart lists (Today, Tomorrow, Next 7 Days) and filters are computed in ttui, not read from your TickTick filters.
- Completed tasks cover the last 7 days.
- Moving a task to the Inbox needs at least one task already in the Inbox (that's the only way the API reveals its id).
- The API allows 100 requests a minute; ttui retries with backoff when it hits the limit.
- dida365.com accounts aren't supported.
- OAuth sign-ins last about 180 days (no refresh token); ttui asks you to sign in again when one expires.

## Files

| Path | Contents |
|---|---|
| `~/.config/ttui/config.toml` | Settings |
| `~/.config/ttui/auth.toml` | Sign-in token (`0600`) |
| `~/.config/ttui/themes/` | Custom themes |
| `~/.cache/ttui/snapshot.json` | Last synced data, for instant startup (`0600`) |

## Development

```sh
make test     # go vet + unit tests (no test touches the real API)
make build    # ./ttui
```

Developer commands read `.env` (see `.env.example`):

- `ttui dev demo`: the UI on built-in mock data, no account needed
- `ttui dev run`: the UI with `.env` settings
- `ttui dev lists`, `ttui dev raw METHOD PATH [JSON]`: API checks with `TTUI_DEV_TOKEN`

Releases are built locally and uploaded by hand:

1. `make test`, then `git tag vX.Y.Z && git push origin vX.Y.Z`
2. `make dist`: [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`) writes four `ttui_<os>_<arch>.tar.gz` archives and `checksums.txt` to `dist/`
3. `gh release create vX.Y.Z dist/ttui_*.tar.gz dist/checksums.txt --title vX.Y.Z` (or GitHub → Releases → Draft a new release → attach those five files → Publish). Not a draft or pre-release, or `install.sh` won't find it.

`install.sh` downloads them from the latest release.

Planned: see [ROADMAP.md](../ROADMAP.md).
