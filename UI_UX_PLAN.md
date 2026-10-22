# Folio — UI & UX plan

> **Status: Phase 1 and Phase 2 BUILT 2026-10-22 (not yet deployed). Phase 3 not started.** Defaults chosen for the open decisions: system font; thumbnails as a 72 PPI page-1 render cached on disk; the dashboard re-fetches itself every 5s while visible; web delete is a soft delete with Undo.

---

## 1. Where we are today

Folio has five web surfaces. All are server-rendered Go `html/template` pages that share one `style.css`, with a little inline vanilla JS.

| Surface | File | What it does now | Main UX gaps |
|---|---|---|---|
| Dashboard `/` | `internal/viewer/index.html` | A plain list of notebook titles, plus a "theme · mode · v12 · 2026-10-22 15:04" line | No visual preview. Raw version numbers and timestamps. No search. The empty state is one line of text. Nothing appears until you reload. |
| Notebook viewer `/n/{id}` | `internal/viewer/notebook.html` | Live page images over SSE, an orange flash on changed pages, a PDF button, and a view mode `<select>` | You can't tell which page you're on. No outline or navigation for long notes. Blank boxes while PNGs render (slow on the free tier). Swapping an image flickers. No keyboard use. On mobile the header wraps into a messy block. |
| Log in / Sign up | `internal/account/page.html` | A form inside a card | No brand moment. No show-password toggle. The submit gives no feedback while argon2 runs. The password rule shows up only as an error. |
| Connect `/connect` | same | Steps for the Claude app and Claude Code, Connected apps, advanced PATs | No sign of whether you're *already* connected. Both clients are stacked in one long card. Nothing tells you what to do once connected. |
| Consent `/oauth/authorize` | same | The "Allow access?" card | Permissions are one sentence. No "Not you?" link. |
| Errors | `http.Error` plain text | "notebook not found" in black on white | Breaks dark mode and gives you no way back. |

**Constraints that every idea must respect**
- No JS framework and no build step: html/template, one CSS file, and small inline scripts.
- CSP: `default-src 'self'`, inline scripts and styles allowed, images only from `'self'` and `data:`. So no CDN fonts or icons. Self-host or inline SVG.
- Dark by default. Every idea has to look right in both themes.
- Existing tests check these strings, so keep them: "Allow access?", "your own computer", "Invalid email or password", the PAT command text, `/connect/apps/{id}/revoke`, `/connect/tokens/{id}/revoke`, and the notebook title on `/`.

---

## 2. Design direction

Inspiration: Linear (quiet dark UI, crisp focus states), Readwise Reader and Google Docs (reading-first, outline sidebar), Vercel and Stripe dashboards (status dots, copy-to-clipboard, relative times), and Notion (friendly empty states).

**Principles**
1. **The pages are the hero.** The chrome stays quiet and low-contrast, and gets out of the way while you read.
2. **Show liveness.** Folio's magic is that Claude writes and the page updates. Make that moment visible and delightful.
3. **One obvious next step on every screen.** Empty dashboard → connect. Connected → try a prompt. Viewing → download.
4. **Calm feedback.** Toasts and inline states, never `alert()`, never a bare text page.

**Design tokens (UI0)**
- Spacing scale: 4 / 8 / 12 / 16 / 24 / 32. Radius: 6 for controls, 10 for cards, 999 for pills.
- Type: serve the **Inter** files Folio already ships in `fonts/inter`, via `@font-face` from `/fonts/…`, so the UI matches the PDFs. System font as fallback.
- Colors: keep the current tokens and add `--success` (live dot), `--warning` (reconnecting, change flash), `--surface-2` (hover), and `--focus` (focus ring).
- One button system: `.btn` (primary), `.btn.secondary`, `.btn.ghost`, `.btn.danger`, and an `.icon-btn` for toolbars. All 32px high, with a visible `:focus-visible` ring.
- Motion: 150ms ease-out on hover and fades. Everything is turned off under `prefers-reduced-motion`.

---

## 3. Ideas by surface

Effort: **S** is under 1h, **M** is a few hours, **L** is about a day. **Backend** means Go changes are needed, not just HTML, CSS and JS.

### 3.1 Global

| # | Idea | Why | Effort |
|---|---|---|---|
| G1 | **Brand header:** an inline-SVG notebook logo and the "Folio" wordmark on every page, the same height everywhere, and a user menu (email → Connect, Log out) instead of loose links | Feels like one product, not four pages | S |
| G2 | **Favicon** (SVG data URI) plus `<meta name="theme-color">` per theme | Tabs and mobile address bars look finished | S |
| G3 | **Theme switch as a 3-way icon toggle** (☾ Dark / ☀ Light / ◐ Auto) instead of a `<select>` | Faster, and reads as a setting, not a form | S |
| G4 | **Toast component**: one `toast(msg)` helper in a shared inline script, for "Copied", "Disconnected", "Token revoked" and "Page 4 updated" | Consistent, calm feedback | S |
| G5 | **Styled error pages** (404, 401, 429, 500) using the site layout: an illustration emoji, a short message, and a "Back to notebooks" button | No more plain-text dead ends | S · backend (small helper in viewer and account) |
| G6 | **Accessibility pass**: skip link, `:focus-visible` rings, `aria-live` on status and toasts, labels on icon buttons, and AA contrast in both themes | Basics we must have | S |
| G7 | **Mobile header**: under 640px, collapse the meta items into a "⋯" menu and keep title + main action | The viewer header currently wraps into 3 lines | S |

### 3.2 Dashboard `/`

| # | Idea | Why | Effort |
|---|---|---|---|
| D1 | **Card grid with a page-1 thumbnail** (`/n/{id}/page/1.png`, `loading="lazy"`, a shimmer placeholder until it loads). Title below, with theme and mode as small pills | You recognize notes by how they look | M |
| D2 | **Relative time** ("edited 3 min ago"), with the exact time in a `title` tooltip. Drop the raw `v12` from the card | Human-readable | S |
| D3 | **Search box + sort** (Recent / A–Z), all client-side. Shown once there are more than 6 notebooks. `/` focuses it | Finding notes quickly once you have many | S |
| D4 | **Onboarding empty state**: a 3-step checklist. ① Connect Claude (✓ when a grant or token exists). ② Try a prompt (2–3 sample prompts with copy buttons, e.g. *"Use Folio to make study notes on how DNS works, with a diagram"*). ③ It appears here live | Most new users land on an empty page and don't know what to do | M · backend (has-connection flag) |
| D5 | **Live dashboard**: a new or updated notebook appears or moves to the top without a reload. A tiny `GET /notebooks.json` is polled every 5s while the tab is visible, and new cards get a soft highlight | The "Claude just made this" moment, without switching tabs | M · backend |
| D6 | **Card actions menu (⋯)**: Open, Download PDF, Rename, Delete. Delete shows an "Undo" toast for 8s (it's already a soft delete; add `Undelete`) | Manage notes without asking Claude | M · backend |

### 3.3 Notebook viewer `/n/{id}`

| # | Idea | Why | Effort |
|---|---|---|---|
| V1 | **Toolbar redesign**: ← back · title · **live dot** (green pulse "Live", amber "Reconnecting…", grey "Offline") · **"Page 3 / 12"** indicator · zoom · theme toggle · **Download PDF** primary button | Orientation and status at a glance | S |
| V2 | **No-flicker page swaps**: preload the new PNG with `new Image()` and swap it in only once it has loaded, then crossfade | Today the page goes blank while the render runs | S |
| V3 | **Skeleton pages**: shimmer placeholders at A4 ratio until the first image arrives, plus a "Rendering…" label if it takes over 1.5s | The free tier is slow, so make waiting feel intentional | S |
| V4 | **Page indicator + jump**: an IntersectionObserver updates "Page n / N". Clicking it opens a small "Go to page" input | Long notebooks become navigable | S |
| V5 | **Zoom**: Fit width / 100% / 150% buttons and `+` `-` keys, remembered per browser. Use the page `max-width`; no re-render needed | Reading small code or tables on a laptop | S |
| V6 | **Keyboard shortcuts**: `j`/`k` or `←`/`→` for next/previous page, `g`/`G` for top/bottom, `+`/`-` zoom, `t` theme, `d` download, `o` outline, `?` help overlay | Power-user feel for almost no code | S |
| V7 | **Outline sidebar**: headings with their page numbers. Click to scroll there, and the current section is highlighted. Collapsible, and hidden on mobile behind an "Outline" button. Data: add `outline: [{text, level, page}]` to the SSE `State` (heading blocks + `layout.Result.Blocks[i].Page`) | Study notes are long; this is the biggest reading upgrade | M · backend |
| V8 | **"Follow Claude" mode**: when a page changes, show the toast "Claude updated page 7 · Jump". With a "Follow edits" toggle on, auto-scroll to the changed page. The flash outline becomes a softer glow + "Updated" badge that fades | Watching Claude write live is Folio's best moment | S |
| V9 | **Remember scroll position** per notebook in `localStorage`, and restore it on return | Come back to where you were reading | S |
| V10 | **Empty and deleted states**: "This notebook is empty — ask Claude to add content" with a sample prompt. On delete: an overlay saying "This notebook was deleted" with a back button (instead of only a status word) | Clear dead-end handling | S |
| V11 | **Page numbers under each page** (small, centered) instead of the right-side label that disappears under 1000px | Works on every screen width | S |
| V12 | **Click a page to open a lightbox** at full resolution with pinch-zoom on mobile (native: open the PNG in an `<dialog>` with `overflow:auto`) | Reading diagrams on a phone | S |

### 3.4 Log in / Sign up

| # | Idea | Why | Effort |
|---|---|---|---|
| A1 | **Centered auth layout**: the logo, a one-line value prop, and on sign-up a small 3-icon strip (Sign up → Connect Claude → Notes appear) | A first impression that explains the product | S |
| A2 | **Show/hide password** eye button, and a **Caps Lock is on** hint | Fewer failed logins | S |
| A3 | **Live password hint** on sign-up: "7 more characters" that turns into ✓ | The rule is visible before you submit | S |
| A4 | **Submit feedback**: on submit, disable the button and show "Signing in…" with a spinner | argon2 takes about 100ms+; stops double submits | S |
| A5 | `autofocus` on email; on an error, focus the field and keep the email (already kept) | Small friction removers | S |

### 3.5 Connect `/connect`

| # | Idea | Why | Effort |
|---|---|---|---|
| C1 | **Connection status banner** at the top: "✓ Connected · Claude · active 2 min ago" or "Not connected yet". Uses the grants that already exist | Answers the first question users have | S |
| C2 | **Tabs: Claude app | Claude Code | Other (token)**, with the PAT section moving into "Other". Remember the selected tab | Shorter page, less scrolling | S |
| C3 | **Numbered step UI** with large step circles; the URL copy field becomes a full-width input-style box with an inline copy icon | Easier to follow while switching windows | S |
| C4 | **"After connecting, try:"** 3 sample prompts with copy buttons (shared with D4) | Bridges setup to the first success | S |
| C5 | **Two-step destructive buttons** for Disconnect and Revoke: the first click turns the button red "Confirm?" for 3s, the second click submits | Prevents accidental disconnects without `confirm()` | S |
| C6 | **Relative "last used" times** and an "Active" pill for grants used in the last hour | Shows what's actually in use | S |

### 3.6 Consent `/oauth/authorize`

| # | Idea | Why | Effort |
|---|---|---|---|
| O1 | **App identity row**: a letter avatar from the client name, the name, and the verified host underneath (`claude.ai`), with a "↔" to the Folio logo | Looks like the OAuth screens users trust | S |
| O2 | **Permission checklist**: ✓ Read your notebooks · ✓ Create and edit notebooks · ✓ Delete notebooks, and "You can disconnect any time" | Clear, scannable | S |
| O3 | **"Signed in as x@y · Not you? Switch account"**: log out, then come back to the same authorize URL | Avoids granting with the wrong account | S |
| O4 | **Big full-width Allow** button, plus a secondary Cancel. Keep the loopback warning as a yellow callout | Primary action is obvious | S |

---

## 4. Proposed phases

### Phase 1 — Polish (front-end only, about 1 day)
UI0 tokens, G1–G4, G6, G7, D2, D3, V1–V6, V8–V12, A1–A5, C1–C3, C5, C6, O1–O4.
Only `style.css`, the three templates, and the tiny Go change for C1 (none needed: grants are already passed in).
**Result:** every screen looks and feels finished, the viewer is navigable, and live edits are delightful.

### Phase 2 — Small backend features (about 1 day)
G5 styled errors · V7 outline sidebar · D1 thumbnails · D4 onboarding checklist + C4 prompts · D5 live dashboard · D6 rename/delete/undo from the web.

### Phase 3 — Bigger product features (separate plan each)
- **Version history panel** in the viewer: a list of `Versions()` with tool names ("Claude: append_blocks · 3 min ago") and **Restore** (the store already has `Restore`).
- **Share link**: read-only public link per notebook, with revoke. Needs a security review.
- **Search inside notebooks** (block text) from the dashboard.
- **Tags or folders** for notebooks.
- **PWA**: manifest and install to the home screen, for reading on the phone.

### Not doing (and why)
- A rich in-browser editor: Claude is the editor, and the web is for reading and managing.
- A JS framework, Tailwind or an icon font: the CSP and no-build constraint, and the UI is small enough for vanilla.
- Notification emails: no email infrastructure yet.

---

## 5. Done means (for every task)
- Looks right in **dark and light**, and at **400px** and **1440px** widths.
- Keyboard-only use works, with focus always visible.
- `go test ./...` is green on SQLite and Postgres, and the protected strings in Section 1 are intact.
- No new external requests (CSP unchanged, except `font-src 'self'` if fonts need it).
- Screenshots of each changed screen are checked before committing.

---

## 6. Decisions to make together
1. **Phase 1 scope:** do all of it, or cut items?
2. **Thumbnails (D1):** page-1 PNGs cost a full render per notebook on first load (cached after that). OK on the free tier, or wait?
3. **Fonts:** use Inter for the UI (matches the PDFs), or keep the system font (zero bytes)?
4. **Live dashboard (D5):** polling every 5s (simple), or an SSE stream like the viewer?
5. **Web delete (D6):** allow it from the web, or keep deletion Claude-only?
