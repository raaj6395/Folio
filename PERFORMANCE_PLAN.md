# Folio — Speed & latency plan

> **Status: DRAFT 2026-10-22.** It rests on measurements, not guesses (Section 2). Nothing has been built yet. We agree on it, then build it in the order in Section 5.

---

## 1. Summary

Folio feels slow for four separate reasons. They add up.

| # | Cause | Who feels it | Size |
|---|---|---|---|
| 1 | **CPU starvation.** Render's Free plan gives 0.1 CPU, and every edit runs Typst, which needs about 1 CPU-second per run. | Every edit Claude makes | **10–55 s per edit; the 8-page notebook hits the 20 s timeout** |
| 2 | **Wasted render work.** Each edit runs *two* full Typst compiles in parallel (PDF + layout query), multi-threaded, behind one global lock. | Every edit, and every viewer | 2.3–2.9× slower than needed |
| 3 | **Cold starts.** Free services sleep after 15 min idle and take about 1 min to wake. | The first Claude message after a break ("MCP takes too long to load") | ~60 s, once |
| 4 | **Too many Claude round trips.** Our instructions tell Claude to make 6+ tool calls per notebook, and Claude carries a 27 KB tool list on every turn. | "Claude takes a lot of time" | 3–10 s of model time per extra call |

The fix is mostly *not* new architecture. It's a lean render path (code), enough CPU (hosting), and fewer, bigger tool calls (MCP design).

---

## 2. Measurements

Setup: the production Docker image, run locally with Render's limits (`docker run --cpus=X --memory=512m`) and a real MCP client over HTTP. The test notebook is `testdata/kitchen_sink.json` (36 blocks, 8 pages, diagrams, tables, code).

### 2.1 MCP tool calls today

| Call | 0.1 CPU (Free) | 0.5 CPU (Starter) | 1 CPU (Standard) |
|---|---|---|---|
| `create_notebook` (empty) | **11.1 s** | 1.2 s | 0.4 s |
| `append_blocks` (36 blocks → 8 pages) | **34.7 s** | 6.8 s | 2.2 s |
| `append_blocks` (1 paragraph to the 8-page notebook) | **timeout (20 s)** | 5.8 s | 1.7 s |
| `get_outline` / `list_notebooks` / `export_pdf` | **timeout (20 s)**, because they rebuild after a failed build | ~5 ms | ~3 ms |
| `render_pages [1]` | **timeout** | 1.7 s | 0.7 s |
| Viewer: first page image | — | 2.2 s | 1.0 s |
| `initialize`, `tools/list`, `get_authoring_guide` | 5–70 ms | 5–70 ms | 5–70 ms |

At 0.1 CPU the 8-page notebook **can't be rendered at all**: the build times out, isn't cached, and every later call retries it and times out again. MCP plumbing, auth and the DB are fast. **Typst is ~100% of tool latency.**

### 2.2 Where a render's time goes (8-page notebook, inside the container)

| Typst run | CPU time |
|---|---|
| Trivial document ("hi"), process startup + fonts | 0.15 CPU-s |
| `compile` to PDF | 0.66–0.95 CPU-s |
| `eval` layout query (the positions Claude needs) | 0.58–0.91 CPU-s |
| `compile` to 8 PNGs at 110 ppi | 0.85–1.33 CPU-s |
| `typst watch` incremental recompile after a 1-paragraph change | 0.27 s wall at full CPU (vs 0.70 s full) |

Each edit today = PDF compile + query in parallel, multi-threaded. Under a CPU quota, threads burn the quota and then get throttled.

### 2.3 The lean path, measured (same notebook, same container)

| CPU | Today: PDF + query, parallel, default threads | Lean: query only, `-j 1`, `--ignore-system-fonts` | Speed-up |
|---|---|---|---|
| 0.1 | 54.3 s | 23.2 s | 2.3× |
| 0.5 | 5.6 s | **1.9 s** | 2.9× |
| 1.0 | 1.8 s | **0.7 s** | 2.5× |

Even lean, 0.1 CPU is too slow for real notes. **CPU is the floor; the code fixes multiply whatever CPU we have.**

### 2.4 Network and the browser (from India, against production, warm)

- A request reaches Cloudflare's Chennai edge in ~50 ms, then goes to the origin in Oregon: **~0.3 s per request**.
- `style.css`, `app.js` and `logo.svg` have no cache headers, so each page view revalidates them, paying that 0.3 s again.
- Cloudflare already gzips responses. Our own process boots in **1.8 s**, so almost all of the ~1 min cold start is Render waking the instance.

### 2.5 What Claude carries on every turn

- `tools/list` = **27 KB** (22 tools). **11.6 KB of it is one 913-byte output schema repeated on 13 tools.** That text sits in Claude's context on every turn of every chat with Folio connected.
- The server instructions recommend: `get_authoring_guide → create_notebook → append section by section (N calls) → render_pages → fix → export_pdf`. That's **6–10 model round trips** for one notebook, each a few seconds of model time *plus* our render time.

---

## 3. The plan

Gains marked "measured" come from Section 2. The others are estimates.

### A. Render pipeline (code; the biggest win)

| ID | Change | Gain |
|---|---|---|
| **R1** | **Edits run only the layout query.** Build the PDF lazily, only on `export_pdf`, the viewer's PDF download, or a print, and cache it by layout key. | Measured: **~2.5×** per edit (with R2) |
| **R2** | Typst flags: `--ignore-system-fonts` (we ship our own fonts; the server has none) and `-j N` with `N = runtime.GOMAXPROCS(0)`. Go 1.25+ makes that respect the container's CPU limit, so it's 1 on fractional CPUs. | Part of the measured 2.5× |
| **R3** | **Per-notebook locks** instead of one global `Builder.mu`, check the cache *before* locking, and add a small semaphore (1 render per CPU) so renders queue instead of fighting over the CPU. | One user's render no longer blocks everyone else's viewer, list and outline calls |
| **R4** | **Reads never render.** `list_notebooks` and the notebook info use the cached layout if it exists; if not, they omit `page_count` instead of building. | `list_notebooks` stays ~5 ms even after a restart |
| **R5** | **Timeout and failure handling.** Scale the Typst timeout to the CPU and remember a failed build for its key for 30 s, so retries don't pile up. | No more 20 s timeout cascades |
| **R6** | **Viewer image debounce.** While Claude is writing (versions changing within 1.5 s), the viewer waits for a quiet moment before fetching new page images. | Stops viewer PNG renders from stealing CPU from Claude's edits |

### B. Fewer and lighter Claude round trips (MCP design)

| ID | Change | Gain |
|---|---|---|
| **M1** | `create_notebook` accepts optional `blocks`, so **one call** makes a complete notebook. | −1 to −N calls per notebook |
| **M2** | Rewrite the server instructions for speed: "write the whole notebook in one `create_notebook` call (or one `append_blocks` per large section); only call `render_pages` if a warning mentions overflow or the user asks to see it; `get_authoring_guide` only when unsure." Put a 10-line block cheat sheet directly in the instructions, so the guide call is rarely needed. | Typical notebook: **6–10 calls → 1–3** |
| **M3** | **Remove output schemas from `tools/list`** (structured results still work without them) and tighten long descriptions. | Measured: **27 KB → ~15 KB** of context on every turn |
| **M4** | **Tool annotations** (MCP spec): `readOnlyHint` on read tools (`list_notebooks`, `get_outline`, `read_blocks`, `render_pages`, …) and `destructiveHint` on `delete_notebook` / `delete_blocks`. Clients can use these to treat safe tools differently, for example in permission prompts. | Fewer interruptions (depends on the client) |
| **M5** | Mutation responses stay compact: they already are (0.6–5 KB). Keep `layout.blocks` limited to changed blocks. | — |

### C. Hosting

| ID | Change | Gain |
|---|---|---|
| **H1** | **Move off Free. Minimum: Starter (0.5 CPU); recommended: Standard (1 CPU, 2 GB).** Paid instances also **don't spin down**, which removes the ~1 min cold start. Check current prices on render.com/pricing. | Measured with R1+R2: an 8-page edit takes **1.9 s on Starter** and **0.7 s on Standard**, against 20 s+ today |
| **H2** | Only if we stay on Free for now: a keep-alive ping to `/healthz` every 10 min from an external cron (one service running 24/7 fits in the 750 free hours). | Removes cold starts only, **not** the slowness |
| **H3** | Keep the Oregon region for now. The Claude app calls MCP from Anthropic's servers, not from your laptop. Only the viewer and Claude Code pay the India–US ~0.3 s per request. Moving to Singapore means migrating the DB too; revisit if most use is from Claude Code in India. | — |

### C.1 Free hosting alternatives (researched 2026-10-22)

**Important: Render's free Postgres expires 30 days after it's created.** It then becomes inaccessible, and is deleted with all its data after a 14-day grace period. Free databases have no backups. We must move or upgrade the DB whatever we decide about compute.

| Platform (free tier) | CPU / RAM | Sleeps? | Fits Folio? |
|---|---|---|---|
| Render Free (today) | 0.1 CPU / 512 MB | after 15 min, ~1 min wake | ❌ too little CPU (measured) |
| Koyeb Free | 0.1 vCPU / 512 MB | after 1 h idle | ❌ same CPU as Render |
| Fly.io | no free tier for new accounts (short trial only) | — | ❌ |
| Hugging Face Spaces | CPU Basic 2 vCPU / 16 GB, but Docker Spaces that run compute now need a paid plan | after 48 h | ❌ not free for us |
| **Oracle Cloud Always Free** (Ampere ARM VM) | **2 OCPU / 12 GB** total since mid-2026 (was 4 / 24) | no | ✅ **~20× Render's CPU.** We run it ourselves |
| **Google Cloud Run** (free monthly quota) | 1+ vCPU per instance; 180k vCPU-s + 360k GiB-s + 2M requests free | scales to zero, wakes in seconds | ✅ real CPU per request. Needs a billing account |
| Google Compute Engine e2-micro (Always Free, US regions) | 2 shared vCPU at ~25% baseline, bursting / 1 GB | no | ⚠️ bursts help short edits. Verify current terms |

Free Postgres to pair with them: **Neon Free** (0.5 GB per project, 100 CU-hours/month, scales to zero after 5 min with a sub-second wake) or **Supabase Free** (pauses after 7 days of low activity). Or run Postgres on the Oracle VM itself with our existing `docker-compose.yml`.

**Option 1: Oracle Always Free (best free performance).**
- **Setup:** one ARM VM in the Mumbai or Hyderabad region (close to you), running Folio + Postgres with our `docker-compose.yml`. Caddy provides automatic HTTPS on a free DuckDNS name, or a ~$1/yr domain; Claude needs an HTTPS URL.
- **Expected speed:** at least the 1-CPU numbers in Section 2.3 (an 8-page edit in about 0.7 s with R1+R2). There's no sleep, and the viewer is fast from India.
- **Catches:**
  - A card is needed to sign up.
  - ARM capacity is sometimes "unavailable" in popular regions.
  - Oracle can reclaim Always Free instances that stay mostly idle for 7 days. Converting the account to Pay-As-You-Go (still $0 within the free limits) is the usual way to avoid that.
  - The 2026 cut from 4 to 2 OCPU shows the limits can change without notice.
  - **We become the ops team:** OS updates, backups (a nightly `pg_dump` to object storage), firewall.
- **Code change:** the Dockerfile downloads the x86 Typst binary; it must choose `aarch64` on ARM (a one-line `TARGETARCH` switch).

**Option 2: Google Cloud Run + Neon (no servers to manage).**
- **Setup:** our image runs as is; it already reads `$PORT` and uses `/tmp`.
- **Expected speed:** 1 vCPU per instance, so 1-CPU speed for edits. A cold start is seconds, not a minute.
- **Catches:**
  - Request-based billing counts the time any request is open. An open viewer tab (SSE) keeps the instance billed, so ~180k vCPU-s is about 50 hours of open viewer tabs per month at 1 vCPU. After that it costs ~$0.09 per vCPU-hour.
  - The disk is in-memory and wiped on scale-down.
  - It needs a billing account.
  - The free-tier region rules are unclear in the sources; verify before choosing `asia-south1`.

**Option 3: stay on Render and pay** (Starter 0.5 CPU or Standard 1 CPU) with Neon Free for the DB. No migration and no ops, but not free.

### D. Web app

| ID | Change | Gain |
|---|---|---|
| **W1** | Versioned static URLs (`/style.css?v=<hash>`) with `Cache-Control: public, max-age=31536000, immutable` for CSS, JS and the logo. | Saves ~0.3 s per asset per page view from India |
| **W2** | Viewer SSE: the poll interval goes from 500 ms to 1 s, and it skips rebuilding state when the version didn't change. It already does the latter; check it stays cheap with many tabs open. | Less DB load |

### E. Later, only if the numbers above aren't enough

- **E1: warm Typst (incremental compilation).** Keep a long-lived compiler per active notebook. `typst watch` recompiled a 1-paragraph change in **0.27 s vs 0.70 s** (measured). Doing this properly means a small Rust sidecar using the `typst` crate, with fonts loaded once and caches kept between edits. That's the biggest remaining render win, but a real project.
- **E2: persist the layout cache in Postgres.** `/tmp` is wiped on every deploy, so the first touch of each notebook after a deploy re-renders it.
- **E3: separate render worker.** See Section 4.

---

## 4. Your two questions

### "Can we separate the backend and frontend to make it lighter?"

**Not worth it now; it wouldn't make anything faster.** The whole frontend is about 30 KB (HTML templates, 20 KB of CSS, 8 KB of JS), and Cloudflare already compresses it. None of the slowness is in serving pages: it's Typst. A split adds a second deploy, cross-origin cookies and CORS, and a second cold start. **W1 (static caching) gets the frontend benefit in about 10 lines.**

### "Separate backend for MCP that handles Typst?"

**It's the right shape eventually, but the wrong first step.** Reasoning:

- **It doesn't add CPU by itself.** On Render Free it's just another 0.1 CPU box with its *own* 1-minute cold start, plus a network hop. Today it would make things slower.
- **It needs shared storage.** Renders read `doc.json`, fonts and diagram SVGs, and produce PDFs and PNGs. A separate worker needs those sent over the wire or stored in shared storage (S3/R2), and results sent back.
- **What it *is* good for:**
  1. Isolating CPU spikes, so MCP auth, lists and the viewer stay snappy while renders run.
  2. Scaling renders separately from the web app.
  3. Running renders on burst, per-second-billed CPU (Cloud Run, Fly Machines) instead of paying for an always-on big instance.
- **The plan gets #1 inside one process for free:** R3's per-notebook locks plus the render semaphore isolate rendering without a network hop. And the render boundary already exists in the code (`layout.Builder`: `Build`, `RenderPNG`), so moving it out later is mechanical.
- **When to split:** if, after R1–R6 and H1, render queue waits regularly go above ~1 s, or we need more than 1 CPU only for rendering. Then E1 (a warm Rust Typst sidecar) is the natural thing to turn into its own service.

---

## 5. Build order and targets

| Step | Items | Effort | Target, measured with the bench in Section 6 |
|---|---|---|---|
| 1 | **R1, R2, R4, R5** | ~half a day | Edit to the 8-page notebook: ≤ 2.0 s at 0.5 CPU, ≤ 0.8 s at 1 CPU. `list_notebooks` < 50 ms always |
| 2 | **M1, M2, M3, M4** | ~half a day | `tools/list` ≤ 16 KB. A typical "make notes on X" takes ≤ 3 tool calls |
| 3 | **R3, R6, W1, W2** | ~half a day | A viewer open during writes adds < 20% to edit latency; static assets are cached |
| 4 | **H1** (your decision and account) | 10 min | No cold starts; numbers from step 1 in production |
| 5 | E1–E3 | only if needed | — |

**Done means:**
- The bench shows the targets.
- `go test ./...` and `make test-pg` are green.
- The outputs stay identical: the same PDF and PNG content, the same layout numbers, and the existing layout tests unchanged.

## 6. How we prove it

Add `make bench CPUS=0.5`. It builds the production image, runs it with `--cpus=$(CPUS) --memory=512m`, signs up, and times these calls:
- `create_notebook`
- the kitchen-sink `append_blocks`
- a 1-paragraph append
- `get_outline`, `list_notebooks` and `render_pages`
- the first viewer image

It prints the same table as Section 2.1. We run it before and after each step and paste the numbers into this file.

## 7. Decisions for you
1. **Hosting (H1 / C.1):** Oracle Always Free (best free CPU, we run the server), Cloud Run + Neon (no server management, nearly free for light use), or a paid Render plan? **Whichever we pick, the Render free Postgres must move or be upgraded before it expires.**
2. **Instructions (M2):** OK for Claude to default to *not* rendering page previews unless there's a layout warning or you ask? That's faster, but Claude checks its work visually less often.
3. **Start with step 1 + step 2 together?** They're independent and both are pure code changes.
