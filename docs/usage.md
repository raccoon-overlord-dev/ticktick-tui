# ttui usage

Everything beyond the [README](../README.md): install options, sign-in, keys, quick add, configuration, themes, limits.

## Install

macOS and Linux (Arch/Omarchy, Debian/Ubuntu/Pop!_OS, Fedora). On Windows, use WSL.

```sh
curl -fsSL https://raw.githubusercontent.com/raccoon-overlord-dev/ticktick-tui/main/install.sh | sh
```

The script downloads the latest release for your OS and CPU, checks its SHA-256 against `checksums.txt`,
and installs `ttui` to `~/.local/bin` (no sudo). If that folder isn't on your `PATH`, it prints the line to add.
Run it again to upgrade.

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
| `h` `l` / `←` `→` | Previous / next pane; `l` opens a list, a folder or a task |
| `1` `2` `3`, `tab` | Focus lists / tasks / details, cycle panes |
| `⏎` | Open; on a detail field: edit it, or cycle Repeat / Priority, or tick a checklist item |
| `x` / `space` | Complete or reopen the task (right after completing, `x` undoes). In lists, `space` folds a folder |
| `p` | Cycle priority High → Medium → Low → None |
| `i` / `e` | Edit the title (tasks) or the field under the cursor (details). `esc` or `⏎` saves; in notes `⏎` is a newline and `esc` / `ctrl+s` saves (`ctrl+⏎` also saves where the terminal passes it through; Omarchy uses it for fullscreen) |
| `a` / `n` | Quick add |
| `/`, `ctrl+k` | Search |
| `:` | Commands |
| `@` / `#` | Jump to a list / tag |
| `t` | Toggle due-date labels |
| `ctrl+r` | Sync now |
| `,` | Settings |
| `esc` / `q` | Back / quit |

### Command bar

The first character picks the mode: nothing = search, `>` commands, `+` quick add, `@` lists, `#` tags.

Quick add understands `call mom tomorrow 17:00 !high #errand @home`:

- priority `!high` `!med` `!low` (or `!h` `!m` `!l`, `!3` `!2` `!1`)
- day `today`, `tomorrow`/`tmr`, weekday names (`fri` = next Friday), `2026-10-08`; time `17:00` or `5pm` (a time alone means today)
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
| `tasks` | `due_label`, `sort_in_priority`, `week_start` | `true`/`false`; `due`·`title`·`created`; `mon`·`sun` |
| `keys` | `keymap` | `vim+arrows` · `arrows` |
| `account` | `sync_every` | `1m` · `5m` · `15m` · `manual` |

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
- Tasks can't be moved between lists yet.
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

1. `git tag v0.1.0 && git push origin v0.1.0`
2. `make test && make dist`: [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`) writes four `ttui_<os>_<arch>.tar.gz` archives and `checksums.txt` to `dist/`
3. GitHub → Releases → Draft a new release → pick the tag → attach those five files → Publish

`install.sh` downloads them from the latest release.

Planned: see [ROADMAP.md](../ROADMAP.md).
