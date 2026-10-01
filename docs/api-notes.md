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
| Trash | No endpoint (`/trash` 404). `GET /project/trash/data` returns `200 {}`, but so does any unknown id (`/project/<anything>/data` → `200 {}`). Deleted tasks disappear from `/data` and `/task/filter`, yet `GET /project/{pid}/task/{tid}` still returns them (no `deleted` field). | Hide Trash. Never use "GET by id succeeds" as proof a task exists. |
| Attachments | No attachment field in any task response. | Hide Files field and meta icon. |
| Signed-in email | No `/user` endpoint (`/user`, `/user/profile` 404). `POST /preference` only returns `{timeZone}` (`GET /preference` → 500). **But** `GET /project/{id}/members` (works with `inbox` too) returns `[{username, displayName, self: true}]` where `username` is the account email. | Show email from the `self` member of `/project/inbox/members`. One extra request at sign-in. |
| Token lifetime / refresh | OAuth token response (tested 2026-09-30): `access_token`, `token_type: bearer`, `scope: tasks:read tasks:write`, `expires_in: 15551999` (~180 days). **No `refresh_token`.** | Store `expires_at`. On 401 go to the auth screen with "Session expired". |
| Rate limits | **Hard cap: 100 requests/minute per account.** Over the cap the API returns **HTTP 500** (not 429) with body `{"errorCode":"exceed_query_limit","errorMessage":"Query rate limit exceeded. Maximum 100 requests per minute. ..."}`. Took about a minute to recover. | Treat `500` + `exceed_query_limit` as rate limiting: back off ≥ 60 s. Keep request count low (see "Sync strategy"). |
| dida365 | `api.dida365.com/open/v1` and `dida365.com/oauth/authorize` exist and respond. dida365 is a separate account system with its own developer portal; our client id is registered on ticktick.com only. Not tested end-to-end (no dida365 account). | Hide the server toggle for v1. |
| Reopen | `POST /task/{id}` with `{"id","projectId","status":0}` reopens a completed task. `completedTime` is **not** cleared. | Support reopen. Ignore `completedTime` when `status == 0`. |

## Other findings

- **`POST /task/filter` with `{"status":[0]}` returns every open task in every list, Inbox included, in one request.** Same ids and same fields as calling `/project/{id}/data` for each list. `{}` gives the same result (open tasks only).
- **Sync strategy** (instead of fetching `/data` for every list): startup and each sync = `GET /project` + `GET /project/group` + `POST /task/filter` = 3 requests, regardless of list count. `sync_every = 1m` fits within the rate limit.
- `GET /tag` returned `[]` even while a task had a tag. The tag list must be built from the tasks' `tags` arrays.
- **Folders:** `GET /project/group` returns `[{id, name, sortOrder, showAll}]`. A list inside a folder carries `groupId` equal to the folder `id`; lists outside folders omit `groupId`. Folders hold no tasks themselves. `closed` was absent on every list (treat missing as `false`). Lists and folders both have `sortOrder`; confirm the sort direction against the web app in Phase 3.
- Dates come back **with milliseconds**: `2026-10-02T09:00:00.000+0000`. We send them without (`...T09:00:00+0000`). The parser must accept both.
- Setting `dueDate` also sets `startDate` to the same value.
- **Partial updates work:** `POST /task/{id}` with only `id`, `projectId` and one field keeps all other fields (tags, items, content, dueDate).
- Checklist toggle: send the full `items` array with the changed `status` (item statuses: `0` open, `1` done).
- **Completing a recurring task** (`repeatFlag` set) keeps the same task id open with `dueDate` moved to the next occurrence, and creates a separate completed copy (new id, no `repeatFlag`) that appears in `/task/completed`.
- `POST /task/move` takes `[{fromProjectId, toProjectId, taskId}]` and returns the moved tasks.
- `POST /task/completed` takes `{projectIds?, startDate, endDate?}`; `POST /task/search` takes `{keywords, projectIds?, tags?, status?, dueFrom?, dueTo?}` (field is `keywords`, not `keyword`).
- `content` round-trips markdown as-is. Task `desc` and `items` are omitted when empty.
- Extra task fields seen: `columnId`, `columnName`, `progress`, `isFloating`, `createdTime`, `modifiedTime`, `etimestamp`.

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
- **Edits to a deleted task return 200** with the task body, so a failed write can't be detected that way.
- **Checklist item status** is `0` open / `1` done in writes; send the whole `items` array.
