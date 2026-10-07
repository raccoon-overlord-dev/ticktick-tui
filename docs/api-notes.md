# TickTick Open API v1 — spike findings

Tested 2026-09-30 against `https://api.ticktick.com/open/v1` with a personal API token.
Reference client: `@ticktick/ticktick-cli` v0.1.14 (`dist/lib/api.js`, `oauth.js`).
Reproduce with `ttui dev raw METHOD PATH [JSON]` (reads `TTUI_DEV_TOKEN` from `.env`).

## Open questions answered

| Item | Finding | Decision |
|---|---|---|
| Inbox | Not in `GET /project`. Real id is `inbox<userId>` (numeric user id). `GET /project/inbox/data` works as an alias (returns `tasks`, `columns`, no `project`). `GET /project/<realInboxId>/data` also works; `GET /project/<realInboxId>` is 404. `POST /task` with no `projectId` lands in Inbox. | Add a synthetic Inbox row. Treat any `projectId` starting with `inbox` as Inbox; learn the real id from the first Inbox task. |
| Smart lists | No endpoint. | Client-side, as planned. |
| Filters | No saved-filter endpoint. | Hard-coded client-side filters, as planned. |
| Trash | No endpoint (`/trash` 404). `GET /project/trash/data` returns `200 {}`, but so does any unknown id (`/project/<anything>/data` → `200 {}`). Deleted tasks disappear from `/data` and `/task/filter`, yet `GET /project/{pid}/task/{tid}` still returns them (no `deleted` field). | Hide Trash. Never use "GET by id succeeds" as proof a task exists. **Undelete doesn't work either** (tested 2026-10-07 on a deleted task): `POST /task/{id}` with a field change, with `"deleted": 0` or with `"status": 0`, complete then reopen, `POST /task/move` within the Inbox or to another list all return 200, yet the task never comes back in `/data` or `/task/filter`. `POST /task` with the old `id` creates a new task with a new id. ttui keeps a local Trash of tasks deleted from it (`trash.json` in the cache) and restores by creating the task again. |
| Attachments | No attachment field in any task response. | Hide Files field and meta icon. |
| Signed-in email | No `/user` endpoint (`/user`, `/user/profile` 404). `POST /preference` only returns `{timeZone}` (`GET /preference` → 500). **But** `GET /project/{id}/members` (works with `inbox` too) returns `[{username, displayName, self: true}]` where `username` is the account email. | Show email from the `self` member of `/project/inbox/members`. One extra request at sign-in. |
| Token lifetime / refresh | OAuth token response (tested 2026-09-30): `access_token`, `token_type: bearer`, `scope: tasks:read tasks:write`, `expires_in: 15551999` (~180 days). **No `refresh_token`.** | Store `expires_at`. On 401 go to the auth screen with "Session expired". |
| Rate limits | **Hard cap: 100 requests/minute per account.** Over the cap the API returns **HTTP 500** (not 429) with body `{"errorCode":"exceed_query_limit","errorMessage":"Query rate limit exceeded. Maximum 100 requests per minute. ..."}`. Took about a minute to recover. | Treat `500` + `exceed_query_limit` as rate limiting: back off ≥ 60 s. Keep request count low (see "Sync strategy"). |
| dida365 | `api.dida365.com/open/v1` and `dida365.com/oauth/authorize` exist and respond. dida365 is a separate account system with its own developer portal; our client id is registered on ticktick.com only. Not tested end-to-end (no dida365 account). | Hide the server toggle for v1. |
| Reopen | `POST /task/{id}` with `{"id","projectId","status":0}` reopens a completed task. `completedTime` is **not** cleared. | Support reopen. Ignore `completedTime` when `status == 0`. |

## Other findings

- **`POST /task/filter` with `{"status":[0]}` returns open tasks from every list, Inbox included, in one request, but at most 200.** Same ids and same fields as `/project/{id}/data`. `{}` gives the same result (open tasks only). Found 2026-10-02 on a large account: it returned exactly 200 and left whole lists out, with no paging field or header. `GET /project/inbox/data` works for the Inbox.
- **Sync strategy:** `GET /project` + `GET /project/group` + `POST /task/filter` (+ `/task/completed` and the Inbox members for the email). If the filter returns 200 tasks (`api.FilterCap`), it was cut short: ttui then reads `/project/{id}/data` for the Inbox and every list instead (6 at a time; rate-limited requests are retried). That is about 1 request per list per sync, so with ~90 lists `sync_every = 1m` can reach the 100/minute limit. `/task/completed` may have the same cap (untested); if an answer is full, ttui splits the date range in two and asks again. Its `endDate` does filter (checked 2026-10-06).
- **"Show in smart list" is not exposed.** Tested 2026-10-02 with a list set to "Do not show": `GET /project`, `GET /project/{id}` and `/project/{id}/data` return the same fields as before (`id, name, sortOrder, groupId, viewMode, kind`, no `inAll` or similar), and `POST /task/filter` still returns that list's tasks. ttui keeps its own setting instead (`smart_hidden`, toggled with `H`).
- `GET /tag` returned `[]` even while a task had a tag. The tag list must be built from the tasks' `tags` arrays.
- **Folders:** `GET /project/group` returns `[{id, name, sortOrder, showAll}]`. A list inside a folder carries `groupId` equal to the folder `id`; lists outside folders omit `groupId`. Folders hold no tasks themselves. `closed` was absent on every list (treat missing as `false`). Lists and folders both have `sortOrder`; confirm the sort direction against the web app in Phase 3.
- Dates come back **with milliseconds**: `2026-10-02T09:00:00.000+0000`. We send them without (`...T09:00:00+0000`). The parser must accept both.
- Setting `dueDate` also sets `startDate` to the same value.
- **Partial updates work:** `POST /task/{id}` with only `id`, `projectId` and one field keeps all other fields (tags, items, content, dueDate).
- Checklist toggle: send the full `items` array with the changed `status` (item statuses: `0` open, `1` done).
- **Completing a recurring task** (`repeatFlag` set) keeps the same task id open with `dueDate` moved to the next occurrence, and creates a separate completed copy (new id, no `repeatFlag`) that appears in `/task/completed`.
- `POST /task/move` takes `[{fromProjectId, toProjectId, taskId}]` and returns the moved tasks. ttui ignores the response and re-reads the task with `GET /project/{to}/task/{id}`.
- New checklist items can be sent without an `id` inside the full `items` array; the server assigns ids (used by `c`, verified in v0.2.0).
- The Inbox's real project id (`inbox<user id>`) is only visible on tasks in it; there is no endpoint for it.
- `POST /task/completed` takes `{projectIds?, startDate, endDate?}`; `POST /task/search` takes `{keywords, projectIds?, tags?, status?, dueFrom?, dueTo?}` (field is `keywords`, not `keyword`).
- `content` round-trips markdown as-is. Task `desc` and `items` are omitted when empty.
- **Checklist tasks** (`kind: "CHECKLIST"`) keep their description in `desc`, with `content` empty. Text tasks use `content`. ttui reads and writes whichever field matches `kind` (writing `desc` verified in v0.3.0).
- Extra task fields seen: `columnId`, `columnName`, `progress`, `isFloating`, `createdTime`, `modifiedTime`, `etimestamp`. ttui reads `createdTime` and `modifiedTime` for grouping and sorting (`s`).
- There is no API for the web app's per-list sort settings (Group by / Sort by / Order), so ttui keeps its own in `config.toml` (`[tasks.list_sort]`); they don't sync with the web app.

## OAuth (Phase 2)

- Our registered app accepts **PKCE without `client_secret`**: `POST https://ticktick.com/oauth/token` with form fields `code, client_id, code_verifier, grant_type=authorization_code, scope, redirect_uri`. The secret is never needed and never shipped.
- The redirect URI registered in the developer portal ("OAuth redirect URL") is `http://localhost:8421/callback`.
- Over SSH, the browser runs on another machine: forward the port with `ssh -L 8421:127.0.0.1:8421 <host>`.

## Writes (Phase 4)

- **Clearing a due date:** send `"dueDate": null` (and `"startDate": null`). An empty string `""` is silently ignored. Clearing the due date also clears the repeat rule.
- **Clearing a repeat rule:** `"repeatFlag": ""` works.
- **A repeat rule needs a due date:** setting `repeatFlag` on a task without one returns 200 but drops the rule. ttui asks for a due date first.
- **All-day dates:** send local midnight with the offset plus `isAllDay: true` and the IANA `timeZone` (e.g. `2026-10-02T00:00:00+0200`, `Europe/Rome`); the server stores it as UTC (`2026-10-01T22:00:00.000+0000`).
- **Completing a repeating task** (`POST .../complete`) returns no body; re-read the task (`GET /project/{pid}/task/{id}`) to get the next occurrence.
- **Repeat rules** (tested 2026-10-02: create with `repeatFlag`, complete 3 times, read the next `dueDate`). The server stores the string as sent and computes every next occurrence itself, so any of these works through the Open API:
  - RRULE: `FREQ=DAILY|WEEKLY|MONTHLY|YEARLY;INTERVAL=n`, `BYDAY=MO,WE,FR`, `BYMONTHDAY=23`, `BYMONTHDAY=-1` (last day), `BYDAY=3WE` or `BYDAY=WE;BYSETPOS=3` (3rd Wednesday), `BYDAY=MO,TU,WE,TH,FR;BYSETPOS=1` / `-1` (first / last weekday of the month, holidays not considered).
  - End: `COUNT=n` (decremented on each completion; the last one leaves the task completed) and `UNTIL=YYYYMMDD`.
  - TickTick extensions: `TT_SKIP=HOLIDAY,WEEKEND` (skips weekends; which holiday calendar is unknown), `ERULE:NAME=FORGETTINGCURVE;CYCLE=0` (Ebbinghaus: +1, +1, +2 days…), `ERULE:NAME=CUSTOM;BYDATE=20261005,20261012` (specific dates).
  - `"repeatFrom": "1"` repeats from the completion date instead of the due date. The web app writes `"0"` for due-date repeats; a task created through the API without it comes back with `"2"` after the first completion.
  - What the web app writes for its Custom options (read back 2026-10-02): 1st Monday of the month `RRULE:FREQ=MONTHLY;INTERVAL=1;BYDAY=1MO`; last workday of the month `RRULE:FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=-1;TT_WORKDAY=-1` (not `BYSETPOS`; `BYMONTHDAY=1;TT_WORKDAY=1` gives the first workday, checked by completing it); specific dates `ERULE:NAME=CUSTOM;BYDATE=20261002,20261005,…` (the first date included).
- Checked 2026-10-06 by creating, completing three times and reading the next `dueDate`: several month days `BYMONTHDAY=1,15,-1`; yearly `BYMONTH=10;BYMONTHDAY=6` and `BYMONTH=5;BYDAY=2SU`; `BYDAY=5FR` (months without a 5th Friday are skipped); `TT_SKIP=WEEKEND` skips occurrences that fall on a weekend (it doesn't move them). Creating with `"kind": "CHECKLIST"`, `items` and `desc` gives a checklist task with notes in one request.
- **Deleting:** `DELETE /project/{pid}/task/{id}` returns 200 with no body. On a repeating task it deletes the whole series.
- **Deleting one occurrence** has no endpoint (tested 2026-10-02). ttui completes the task (it moves to the next occurrence), then finds the completed copy in `/task/completed` by list, title and old `dueDate` (the copy has a new id, no `repeatFlag` and no link back) and deletes it. On the last occurrence, completing leaves the task itself completed; ttui deletes it.
- **Edits to a deleted task return 200** with the task body, so a failed write can't be detected that way.
- **Converting note ↔ checklist** (tested 2026-10-07): one `POST /task/{id}` with `kind`, `items`, `content` and `desc` switches it. To checklist: `kind: "CHECKLIST"`, `items` without ids (the server assigns them), `content: ""`. To note: `kind: "TEXT"`, `content`, `desc: ""`, `items: []` (an empty array clears the items).
- **Checklist item status** is `0` open / `1` done in writes; send the whole `items` array.
