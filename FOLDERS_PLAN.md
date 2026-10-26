# Folio — Folders plan: organise notebooks like a GoodNotes library

> **Status: F1, F2 and F3 built 2026-10-26** on branch `folder`, with the Section 10 defaults (contents move up on delete, 3 levels, Recent row on, Claude may create folders). F3 delivered: drag a notebook or folder onto a folder or breadcrumb (desktop), Move to… for folders, 6 folder colours, favourites (★ on the card plus a Favourites row at the top level), and Select → move several notebooks at once. Favourites and moving folders are web-only; Claude's tools are unchanged by F3. Imported notebooks (`folio import`) land at the top level. When `auth-email` merges, its `DeleteUser` must also delete `folders`, and its dashboard banner change to `viewer.index()` moves into `viewer.library()`.

---

## 1. The problem in one paragraph

Every notebook a user owns sits in **one flat grid** on the dashboard. That's fine at 5 notebooks, but a student who asks Claude for notes on each lecture has 30 or more within a term, all mixed together: Networks next to Calculus next to interview prep. People already know how to fix this from GoodNotes, Notability and Finder: **folders, folders inside folders, and a path at the top showing where you are.** We'll give Folio the same library, and let **Claude file new notes into the right folder** when it creates them, which none of those apps can do.

---

## 2. Where we are today

| Area | Today | Where in the code |
|---|---|---|
| Dashboard | One grid of cards, newest first. Each card has a thumbnail, title, theme and mode pills, and a date. Its menu has Open, Download PDF, Rename and Delete (with Undo) | `internal/viewer/index.html`, `viewer.index()` |
| Finding a notebook | Search by title and sort (Recent or A–Z), **only shown once there are more than 6 notebooks** | `index.html` lines 41–42, inline script `init()` |
| Live updates | The page fetches `/` every 5 s and swaps the grid when the set of `id:version` keys changes; new cards glow | `index.html` `refresh()` |
| Grouping | **None.** No folders, tags or favourites | — |
| Data | A `notebooks` table with `user_id`, soft delete (`deleted_at`), and a limit of 100 notebooks per user | `internal/store/store.go` (`nbCols`, `Notebooks()`, `MaxNotebooks`) |
| Claude (MCP) | `create_notebook` puts every notebook in the same place; `list_notebooks` returns them all, newest first | `internal/mcpserver/tools.go` |
| Notebook viewer | The ← button always goes back to `/` | `internal/viewer/notebook.html` line 21 |
| Earlier decision | `PRODUCT_PLAN.md` §8 deferred folders "until the typical user has 15 or more notebooks". There's no usage data yet (`folio admin stats` doesn't exist), so this plan goes ahead because you asked for it, and keeps v1 small | `PRODUCT_PLAN.md` line 164 |

---

## 3. What we're building (user stories)

1. **As a student**, I can create a folder (e.g. "Semester 1"), and folders inside it ("Networks", "Calculus"), from the dashboard.
2. I can **open a folder** and see its sub-folders first, then its notebooks, with a **path at the top** (`Notebooks › Semester 1 › Networks`) I can click to go back up.
3. I can **move a notebook** into a folder from its card menu ("Move to…").
4. I can **rename** and **delete** a folder. Deleting never loses notes (see 4.4).
5. **When I tell Claude** "make notes on TCP in my Networks folder", the notebook is created there, and the folder is created if it doesn't exist yet.
6. Notebooks Claude makes **without** a folder still appear where I'll notice them (see 4.3), so nothing gets lost.
7. Search finds a notebook **in any folder**, and shows which folder it's in.

### Not in v1 (Section 9)
Drag and drop, folder colours or covers, favourites, moving folders, multi-select, tags, sharing a folder.

---

## 4. The design

### 4.1 Dashboard layout

```
┌────────────────────────────────────────────────────────────────┐
│ Folio                                         ◐ ☾ ☀   (A)      │
├────────────────────────────────────────────────────────────────┤
│ Notebooks › Semester 1                 [ + New folder ] [ ⌕ ]  │
│                                                                │
│ Folders                                                        │
│ ┌──────────┐ ┌──────────┐                                      │
│ │  ▭▭      │ │  ▭▭      │                                      │
│ │ Networks │ │ Calculus │                                      │
│ │ 6 notes  │ │ 3 notes  │                                      │
│ └──────────┘ └──────────┘                                      │
│                                                                │
│ Notebooks                                    Sort: Recent ▾    │
│ ┌──────────┐ ┌──────────┐ ┌──────────┐                         │
│ │ [thumb]  │ │ [thumb]  │ │ [thumb]  │                         │
│ │ Syllabus │ │ Timetable│ │ Exam tips│                         │
│ └──────────┘ └──────────┘ └──────────┘                         │
└────────────────────────────────────────────────────────────────┘
```

- **Folders come first**, then notebooks, the same order GoodNotes and Finder use. Folder cards are shorter than notebook cards (no thumbnail): a folder icon, the name, and "N notes" (counting everything inside, sub-folders included).
- **Path (breadcrumb)** at the top. Each part is a link. At the root it just says "Notebooks".
- **"+ New folder"** creates a folder **in the folder you're looking at**. It opens the same `<dialog>` pattern as Rename.
- **Folder card menu (⋯):** Open, Rename…, Delete.
- **Notebook card menu:** adds **Move to…** between Rename and Delete. It opens a dialog listing every folder as an indented tree, plus "Notebooks (top level)". The current folder is disabled.
- **Search** is always visible once the user has any folders (today it appears only after 6 notebooks). While a search term is typed, the grid shows matches **from every folder**, each with its folder path under the title, and folder cards are hidden.
- **Empty folder:** "This folder is empty. Ask Claude to make notes here, for example: *Make Folio notes on TCP in my Networks folder.*" with a Copy button, reusing the existing `.copy.prompt` style.
- **Phones:** the grid already wraps. The breadcrumb collapses to `‹ Semester 1` (a link to the parent) below 480 px.

### 4.2 URLs

| URL | Shows |
|---|---|
| `/` | The top level: top-level folders, then notebooks that aren't in a folder |
| `/f/{folder_id}` | That folder |
| `/n/{id}` | A notebook (unchanged). Its ← button goes back to the notebook's folder (`/f/{id}`), or `/` |

The 5-second `refresh()` must fetch the **current** URL (`location.pathname`), not `/`.

### 4.3 Where Claude's notebooks land

- With `folder` given: in that folder (created if missing, Section 5).
- Without it: at the top level, as today.
- The top level gets a **"Recent" row** above the folders: the 4 most recently edited notebooks **from any folder**. This solves the problem of "Claude just made it, where did it go?", and that row is where the live "fresh" glow shows a new notebook. Show the row only once the user has at least one folder, so nothing changes for users who don't use folders.

### 4.4 Deleting a folder (safe by default)

- **Delete folder** removes the folder and **moves everything in it (notebooks and sub-folders) up to the parent folder.** No notebook is ever deleted by deleting a folder.
- Confirm in the button ("Click again to delete", the existing `data-confirm` pattern). The toast says "Deleted folder "Networks". Its 6 notebooks moved to Semester 1."
- No Undo is needed, because nothing is lost. (Option B in Section 10 is the GoodNotes behaviour: delete the contents too, with Undo.)

### 4.5 Rules and limits

| Rule | Value | Why |
|---|---|---|
| Name | 1–80 characters, trimmed; no `/` (Claude uses `/` for paths) | Keeps paths unambiguous |
| Unique names | Unique **among siblings**, ignoring case (two "Networks" folders in the same parent → error "A folder with that name is already here") | Paths must resolve to exactly one folder |
| Depth | **At most 3 levels** (e.g. `Semester 1 / Networks / Labs`) | Short breadcrumbs on phones; simple queries; GoodNotes allows more, but students rarely need it |
| Count | At most 200 folders per user | Same spirit as `MaxNotebooks = 100` |
| A notebook | Is in **zero or one** folder | Like files, not tags |
| Moving or filing | **Doesn't change** `updated_at` or `version`, and isn't a snapshot | Filing isn't an edit of the notes: it mustn't reorder "Recently edited", appear in `list_versions`, or be undone by `undo` |

---

## 5. Claude (MCP) changes

Claude should talk about folders by **path** (`"Semester 1/Networks"`), not by ID. That's what users say, and it saves Claude a lookup call.

| Tool | Change |
|---|---|
| `create_notebook` | New optional `folder` (string path). Missing folders along the path are created. Errors if the path is deeper than 3 levels or a name breaks the rules |
| `list_notebooks` | Each notebook gets `"folder": "Semester 1/Networks"` (empty at the top level). New optional `folder` filter (that folder and everything below it) |
| **`move_notebook`** (new) | `notebook_id`, `folder` (path; `""` = top level). Creates missing folders. Add it to the isolation matrix in `auth_test.go` (the test fails automatically if a tool taking `notebook_id` is missing) |
| **`list_folders`** (new) | Returns every folder as `{path, notebook_count}`. Lets Claude answer "which folders do I have?" and pick an existing folder instead of making a near-duplicate |

- Path matching ignores case and extra spaces around `/`, so "semester 1 / networks" finds "Semester 1/Networks".
- **Not exposed to Claude in v1:** renaming or deleting folders. They're rare, easy in the web app, and deleting is the kind of thing an AI shouldn't do by accident. This also keeps the tool list short (every tool costs context in every chat).
- **`guide.md`:** add 3 lines: "If the user names a folder or subject, pass `folder`. Call `list_folders` first if unsure and reuse an existing folder. Don't invent folders the user didn't ask for."

---

## 6. Data model (SQLite and Postgres migrations)

Add one migration to **both** `migrations` and `pgMigrations` in `internal/store/store.go`:

```sql
CREATE TABLE folders (
    id TEXT PRIMARY KEY,               -- "f_" + model.NewID
    user_id TEXT NOT NULL REFERENCES users(id),
    parent_id TEXT REFERENCES folders(id),   -- NULL = top level
    name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX folders_user ON folders(user_id, parent_id);
ALTER TABLE notebooks ADD COLUMN folder_id TEXT REFERENCES folders(id);
CREATE INDEX notebooks_folder ON notebooks(user_id, folder_id);
```

- **Sibling uniqueness is checked in Go**, inside the same transaction as the insert or rename. A `UNIQUE(user_id, parent_id, lower(name))` index behaves differently for `NULL` parents in SQLite and Postgres, so don't rely on it.
- Folders are **hard-deleted** (there's nothing to restore, since contents move up).
- `nbCols` and `scanNotebook` gain `folder_id`; `Notebook` gains `FolderID string`.
- `CreateNotebook` takes a `folderID` (`""` = top level).

### Store API (`internal/store/folders.go`, new file)

| Function | Notes |
|---|---|
| `Folders(userID) ([]Folder, error)` | All of the user's folders in one query (≤ 200 rows); build the tree, paths and counts in Go |
| `CreateFolder(userID, parentID, name) (Folder, error)` | Checks the parent is the user's, the depth, the sibling name and the count |
| `RenameFolder(userID, id, name) error` | Sibling check |
| `DeleteFolder(userID, id) error` | One transaction: `UPDATE notebooks SET folder_id = <parent>`, `UPDATE folders SET parent_id = <parent>`, then `DELETE`. Re-check sibling names when children move up: on a clash, rename the moved child to "Name (2)" |
| `MoveNotebook(userID, notebookID, folderID) error` | **Must check that both the notebook and the folder belong to `userID`.** Doesn't touch `version` or `updated_at` |
| `EnsureFolderPath(userID, path) (Folder, error)` | For MCP: splits on `/`, walks the path or creates it, in one transaction |
| `Notebooks(userID, folderID)` | Add a filter. Keep a way to list all notebooks (search, the Recent row, `list_notebooks` without a filter) |

**Every query includes `user_id = ?`.** That's the whole security model of this feature.

### Other code that touches notebooks
- `store/import.go` copies notebooks from SQLite to Postgres. Copy `folders` too (parents first), or set `folder_id` to NULL and say so in the output.
- The `auth-email` branch adds `DeleteUser`. Whichever branch merges second must add `DELETE FROM folders WHERE user_id = ?` (after notebooks, before users).
- `OpenTestPostgres` in `store/testdb.go`: add `folders` to the `TRUNCATE` list.

---

## 7. Web routes

All go through `s.require(true, …)` and `CrossOriginProtection`, like the existing rename and delete.

| Route | Does |
|---|---|
| `GET /f/{id}` | A folder view (`index()` with a folder; 404 if the folder isn't the user's) |
| `POST /folders` | `name`, `parent_id` → redirect to the parent view |
| `POST /f/{id}/rename` | `name` |
| `POST /f/{id}/delete` | → redirect to the parent with the toast from 4.4 |
| `POST /n/{id}/move` | `folder_id` (`""` = top level) → redirect back to where the user was |

Errors (duplicate name, too deep, too many) use the existing `renderError` page, or better, reopen the dialog with the message.

---

## 8. Build order

| Phase | Delivers | Effort |
|---|---|---|
| **F1: Folders in the web app** | Migration, store API, `/f/{id}` view with breadcrumb, New folder, Rename, Delete (contents move up), Move to…, notebook ← goes back to its folder, search across folders, Recent row, `refresh()` on the current URL | 1.5–2 days |
| **F2: Claude files notes** | `folder` on `create_notebook`, `folder` field and filter on `list_notebooks`, `move_notebook`, `list_folders`, `guide.md` lines, path matching | 0.5–1 day |
| **F3: Polish (later, if used)** | Drag a card onto a folder (desktop), move folders, folder colours, favourites, select several notebooks and move them at once | — |

F1 and F2 can ship together. F2 is the part users will remember ("Claude put it in the right folder").

### Tests the engineer must add (each fails if the rule breaks)
1. **Isolation:** Bob can't see, open, rename, delete or move into Alice's folder. **Bob can't move his own notebook into Alice's `folder_id`.** The `move_notebook` tool is in the isolation matrix.
2. Depth over 3, a duplicate sibling name, `/` in a name, and the 201st folder are all refused.
3. Deleting a folder moves its notebooks and sub-folders to the parent, and no notebook is deleted.
4. Moving a notebook doesn't change its `version`, `updated_at` or `list_versions`, and `undo` doesn't undo the move.
5. `create_notebook` with `folder: "A/B"` creates both folders; a second call reuses them, even with different case or spacing.
6. Both databases: run with `FOLIO_TEST_DB_URL` set (Postgres via `docker compose up`).
7. Dashboard: `/f/{id}` shows the breadcrumb, folders before notebooks, and the empty-folder prompt.

### Done means
- A user can file 30 notebooks into folders and find any of them in 2 clicks or one search.
- "Make Folio notes on X in my Y folder" works in Claude on the first try.
- Nothing changes for a user who never creates a folder (no Recent row, same grid).

---

## 9. Not doing (and why)
- **Tags:** two ways to organise is confusing; folders match what people know from GoodNotes. Revisit if users ask for one notebook in two places.
- **Unlimited nesting:** deep trees hurt on phones, and 3 levels covers "term / subject / topic".
- **Folder colours and covers:** nice, but cosmetic. F3.
- **Shared folders:** needs sharing (`PRODUCT_PLAN.md` Phase 2) first.
- **Letting Claude delete folders:** too easy to do by accident; it's two clicks in the web app.

## 10. Decisions for you
1. **Deleting a folder:** **(A, default)** contents move up to the parent and nothing is lost; or **(B)** delete the contents too, like GoodNotes, with a 9-second Undo toast.
2. **Depth limit:** **3 levels (default)**, 2, or unlimited?
3. **The "Recent" row** at the top level (default: on, once the user has a folder): keep it?
4. **Should Claude be allowed to create folders itself** when the user names one that doesn't exist (default: yes), or only file into existing ones?
