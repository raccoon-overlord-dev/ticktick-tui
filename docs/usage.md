# ttui usage

Everything beyond the [README](../README.md): install options, sign-in, keys, quick add, configuration, themes, limits.

## Install

macOS and Linux (Arch/Omarchy, Debian/Ubuntu/Pop!_OS, Fedora). On Windows, use WSL.

```sh
curl -fsSL https://raw.githubusercontent.com/raccoon-overlord-dev/ticktick-tui/main/install.sh | sh
```

The script downloads the latest release for your OS and CPU, checks its SHA-256 against `checksums.txt`,
and installs `ttui` to `~/.local/bin` (no sudo). If that folder isn't on your `PATH`, it prints the line to add.
Run it again to upgrade, or press `U` in ttui when the status bar shows a new version: ttui checks once a day (Settings → Check for updates, `update_check` in `config.toml`; Settings → Check now looks right away), verifies the download against the release checksums, replaces itself and restarts. If it can't write its own binary (e.g. installed in a root-owned folder), it copies this install command to the clipboard instead.

| Variable | Effect |
|---|---|
| `TTUI_INSTALL_DIR` | Install somewhere else (default `~/.local/bin`) |
| `TTUI_FROM_SOURCE=/path/to/checkout` | Build a local checkout with Go instead of downloading |

Uninstall (asks before deleting your settings and sign-in):

```sh
curl -fsSL https://raw.githubusercontent.com/raccoon-overlord-dev/ticktick-tui/main/install.sh | sh -s -- --uninstall
```

With Go installed: `go install github.com/raccoon-overlord-dev/ticktick-tui/cmd/ttui@latest`, or `make install` from a clone. Run `go install` again to upgrade: these builds report version `dev`, so the update check and `U` are off.

## Sign in

Run `ttui`. The first screen offers two ways:

- **Browser (OAuth)**: ttui opens TickTick's sign-in page and waits on `localhost:8421`. Keys: `o` reopen the browser, `c` copy the URL.
  Over SSH the browser runs on your own computer, so forward the port first: `ssh -L 8421:127.0.0.1:8421 <host>`.
- **API token** (`tab`): paste the token from TickTick web → Settings → Account → API Token.

The token is stored in `~/.config/ttui/auth.toml` (mode `0600`). Sign out from Settings or the command bar.

## Layout

Three panes (lists, tasks, details) at 120 columns or more, two (tasks, details) from 80, one below that,
where details open as a bottom sheet. Force a layout in Settings → Columns.

URLs in task titles, notes (bare `https://…` or markdown `[text](url)`) and checklist items are clickable in terminals
that support OSC 8 hyperlinks (Ghostty, kitty, WezTerm, iTerm2, recent GNOME Terminal and Windows Terminal).
Settings → About shows the version and a link to the project page.

## Keys

Vim keys and arrows both work (Settings → Keymap can switch vim keys off).

| Keys | Action |
|---|---|
| `j` `k` / `↓` `↑` | Move; past the last row you wrap to the first and back. In lists, moving selects the list |
| `g` / `G` | Top / bottom |
| `pgup` / `pgdn` | One page up / down (lists, tasks, long notes, this panel, the notes editor). |
| `h` `l` / `←` `→` | Previous / next pane; `l` opens a list, a folder or a task |
| `1` `2` `3` | Focus the lists / tasks / details pane. With fewer than 3 panes, `1` opens the list jumper (`@`) and `3` opens the details |
| `tab` | Next pane |
| `⏎` | Open; on a detail field: edit it, pick a due date, repeat or list, cycle Priority, or edit a checklist item |
| `x` / `space` | Complete or reopen the task (right after completing, `x` undoes). On a checklist item: tick or untick it; ticking the last open one completes the task and unticking one reopens a completed task, as in the web app. In lists, `space` (or `⏎`, `l`) opens or closes a folder; folders start closed and ttui remembers the open ones |
| `H` | In lists: hide the list from Today, Tomorrow, Next 7 Days and the filters, or show it again (hidden lists get a crossed-out eye icon, `x` without Nerd Fonts). Stands in for TickTick's "Show in smart list: Do not show", which the API doesn't expose |
| `p` | Cycle priority High → Medium → Low → None |
| `d` | Due date menu: the dates in `due_menu`, No date, Pick a date (calendar: arrows move by day / week, `pgup` `pgdn` by month, `home` today, `t` sets a time) or Type a date. A date without a time keeps the task's time |
| `m` | Move the task to another list |
| `c` | Add checklist items: `⏎` adds one and opens the next, `esc` finishes |
| `C` | Convert between a note and a checklist, like the web app. Note → checklist: each line of the notes becomes an item (blank lines are skipped, `- ` / `* ` / `+ ` markers dropped, `- [x] ` lines start ticked). Checklist → note: the notes come first, then one line per item (ticks are lost). Works in the New task panel too |
| `D` / `delete` | Delete the task after a `y / N` prompt. On a repeating task: `o` deletes this occurrence only, `a` the whole series |
| `:` Trash | Tasks deleted with `D` in the last 30 days (whole tasks, not single occurrences), newest first; older ones are dropped automatically. `⏎` restores one to its list (the Inbox if the list is gone) as an open task; **Empty trash** forgets them all. See Known issues |
| `i` / `e` | Edit the title (tasks) or the field under the cursor (details). `esc` or `⏎` saves. On the notes `↑` `↓` scroll them when they are long; while editing notes `⏎` is a newline, `↑` `↓` move between lines and `esc` / `ctrl+s` saves (`ctrl+⏎` also saves where the terminal passes it through; Omarchy uses it for fullscreen) |
| `ctrl+a` | While editing: select all. On a text field (title, notes, checklist item…): edit it with all its text selected |
| `shift+←` `shift+→` | Select text while editing (`shift+↑` `shift+↓` in notes, `shift+home` / `shift+end` too). Typing or pasting replaces the selection, `backspace` deletes it |
| `ctrl+c` / `ctrl+x` | While editing: copy / cut the selection to the clipboard (OSC 52, works over SSH). Never quits while editing |
| `tab` | While editing tags: complete the word you're typing to an existing tag (`#wo` becomes `#work `). Matching tags show under the field and `tab complete` appears in the bottom bar; `↑` `↓` pick which one. Tags the task already has aren't suggested |
| `ctrl+z` / `ctrl+y` | While editing: undo / redo (`ctrl+shift+z` also redoes), a word at a time, until the field is saved. With a terminal that passes `cmd` keys through (kitty keyboard protocol), `cmd+a` / `c` / `x` / `z` / `y` work too |
| `a` | Quick add (one line, see below) |
| `n` | New task panel: the details fields for a task that doesn't exist yet. It opens with the title ready to type (`⏎` saves it, `ctrl+s` creates the task straight away). `⏎` edits or picks each field (due date by calendar or menu, repeat, list, tags, priority, checklist, notes), `ctrl+s` or **Create** adds it, `esc` cancels (asks first if you changed anything). Also in `:` as New task |
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
Settings → Smart dates off (`smart_dates = false`) keeps day and time words in the title; `!`, `#` and `@` still work.

To make a task repeat, give it a due date, then press `⏎` on **Repeat** in the details pane and pick
Daily, Weekdays (Mon–Fri), Weekly, Monthly, Yearly or Never. Weekly, Monthly and Yearly follow the due date
(e.g. "Monthly · on the 23rd").

**Custom** opens the web app's Custom repeat dialog: by due date, by completion date or on specific dates (picked in a calendar);
every N days / weeks / months / years; for weeks the weekdays, for months each chosen day (several, and Last day),
on the first … fifth / last weekday, or the first / last workday; for years a month and day or the Nth weekday of a month;
Skip weekends where the web app offers it. `↑↓` moves, `←→` changes a value, `space` toggles, `ctrl+s` saves.

**Type a rule** (or `i` on Repeat) takes a rule in words, including ends the dialog doesn't have (`x5`, `until`):

| Write | Repeats |
|---|---|
| `every 3 days`, `every 2 weeks`, `monthly`, `yearly` | every N days / weeks / months / years |
| `mon,wed,fri`, `weekdays`, `weekends`, `every 2 weeks on mon,fri` | on those weekdays |
| `23rd` (or `monthly 23`), `1st,15th,last day` | on those days of the month |
| `every year oct 6th`, `every year may 2nd sun` | on that date / weekday of a month each year |
| `first mon`, `3rd wed`, `5th fri`, `last fri` | on that weekday of the month |
| `first workday`, `last workday` | on the first / last working day of the month |
| `curve` | Ebbinghaus forgetting curve |
| `dates fri 2026-10-22` | on these dates only |

Add any of: `until 2026-12-31` or `x5` (`5 times`) to end it, `skip weekends` / `skip holidays`,
`from completion` to count from when you complete it instead of the due date. Leave it empty for no repeat.
Rules set in the web app show in the same words.

### Reminders and notifications

Give a task a due date, then press `⏎` on **Remind** in the details pane (or the New task panel). Like the web app, a
task can have several reminders; `⏎` on an option turns it on or off and the menu stays open:

- all-day tasks: On the day, 1 / 2 / 3 days early, 1 week early, at the Settings → Default time (`09:00`);
- timed tasks: On time, 5 or 30 minutes, 1 hour or 1 day early;
- **Custom**: all-day tasks pick days or weeks early and a time (`←→` moves 15 minutes, or type `1030`); timed tasks
  pick minutes, hours or days early. The dialog shows the result ("Remind at 09:00 on Oct 6, 2026").

Reminders set in the web or phone app show up the same way, and a bell on the task row marks tasks that have one.
The web app's Constant reminder (a paid feature) isn't supported.

Settings → Notifications (`notify`) lets ttui alert you when a reminder comes due:

- `terminal`: your terminal shows a desktop notification (OSC 9). Works in Ghostty, iTerm2, WezTerm and kitty, also
  over SSH; Ghostty needs `desktop-notifications = true` in its config. Alacritty and Terminal.app don't support it.
- `system`: `notify-send` on Linux, Notification Center on macOS (through `osascript`), for the machine ttui runs on.

ttui only notifies while it's open, and reminders that came due while it was closed aren't sent afterwards.
TickTick's own apps notify you too, so you may get the same reminder twice; that's why it's off by default.

### Date and time

Settings → Date & time sets the time format (`24h` 17:30 or `12h` 5:30pm), the date format (`dd/mm/yyyy`,
`yyyy/mm/dd` or `mm/dd/yyyy`) and the first day of the week (for the calendar). The date format also decides how
the Due field reads a typed date: with `dd/mm/yyyy`, `8/10` is 8 October (the next one, if the year is left out).
`2026-10-08` always works. Quick add only takes `2026-10-08`, so "1/2 cup" in a title stays text.

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
| | `smart_hidden` | lists hidden from Today, Tomorrow, Next 7 Days and the filters, written by ttui when you press `H` |
| `tasks` | `due_label`, `week_start` | `true`/`false`; `mon`·`sun` |
| | `group_by` | `list` · `date` · `created` · `tag` · `priority` · `none` (default for all lists) |
| | `sort_by` | `date` · `created` · `modified` · `title` · `tag` · `priority` (default for all lists) |
| | `order` | `oldest` · `newest` (default for all lists) |
| | `list_sort` | per-list overrides written by `s`; "Use default" in the `s` menu removes one |
| | `due_menu` | dates offered by `d`, in quick-add syntax: `["today", "tomorrow", "+2d", "mon", "+7d"]` (config file only) |
| | `smart_dates` | `true` · `false`: quick add reads day and time words as the due date (Settings → Smart dates) |
| | `completed_days` | `"7"` · `"30"` · `"90"` · `"365"`: days of completed tasks to download (Settings → Completed history) |
| `datetime` | `time_format` | `24h` · `12h` |
| | `date_format` | `dd/mm/yyyy` · `yyyy/mm/dd` · `mm/dd/yyyy` |
| `reminders` | `default_time` | time of the preset reminders on all-day tasks: `07:00` · `08:00` · `09:00` · `10:00` · `12:00` · `18:00` · `20:00` (any `HH:MM` in the file) |
| | `notify` | `off` · `terminal` · `system` |
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

- The API has no Trash and can't undelete, so ttui keeps its own Trash (`:` Trash): copies of the tasks you deleted *from ttui* in the last 30 days, stored in `trash.json` in the cache folder. Tasks deleted in the web or phone app don't show up there. Restoring creates the task again with a new id and its title, notes, checklist, tags, due date, repeat and priority (not its created time or completion), and the original stays in the web app's Trash.
- No attachments: the API doesn't expose them.
- A task's last reminder can't be removed through the API (an empty list is ignored), so ttui says so; remove it in the TickTick app. Removing one of several works. A paid plan allows more reminders per task than the free one (the free plan kept 2); extra ones are dropped by the server.
- Notifications only fire while ttui is open; TickTick's own apps may notify you of the same reminder.
- Smart lists (Today, Tomorrow, Next 7 Days) and filters are computed in ttui, not read from your TickTick filters.
- A list's "Show in smart list" setting isn't readable through the API, so ttui ignores it; press `H` on the list to hide it in ttui instead.
- Completed tasks cover the last 7 days by default; Settings → Completed history (`completed_days`) raises it to 30, 90 or 365.
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

Releases are built locally and published with the [GitHub CLI](https://cli.github.com) (`gh auth login` with a
fine-grained token that has **Contents: Read and write** on the repo):

1. Commit and push `main`, then tag it with the release notes as the tag message and push the tag:
   `git tag -a vX.Y.Z` (opens your editor), then `git push origin vX.Y.Z`
2. `make release`: runs the tests, builds four `ttui_<os>_<arch>.tar.gz` archives and `checksums.txt` in `dist/` with
   [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`), and publishes them as release `vX.Y.Z` with the tag
   message as its notes. It stops if the tree has uncommitted changes, HEAD isn't the tag, or the tag isn't pushed.

`install.sh` downloads them from the latest release.

Planned: see [ROADMAP.md](../ROADMAP.md).
