# Shiguang Study Room · Architecture & Development Guide

This is the developer-facing reference. [AGENTS.md](../AGENTS.md) is navigation only;
mechanism details, parameters and test coverage live here. Read this before changing any
code. For user-facing behaviour see [user-guide.md](user-guide.md) and
[ai-guide.md](ai-guide.md).

## 1. Project layout and tooling

- Python side is managed exclusively with [uv](https://docs.astral.sh/uv/): `uv add` /
  `uv remove`, never pip, never a hand-written `requirements.txt`. `uv.lock` is committed
  (run `uv lock` after dependency changes); `.venv/` and `.env` are not.
- Console entry: `[project.scripts] shiguang-study = "shiguang_study.webapp.__main__:main"`.
- The product itself is one file: `src/app/index.html` (~7.5 MB) — UI + core logic + AI
  logic + embedded PDF.js. Changing the app means editing that file directly.

## 2. HTML single-file anatomy (read before touching the app)

- `<script id="pdfjs-core" | "pdfjs-worker" | "pdfjs-resources">` are base64 /
  gzip+base64 resource blocks. **Never hand-edit them** (equivalent to recompiling PDF.js).
- The whole file has exactly **one** inline `<script>`. Upper half: core app. Lower half:
  AI module. The AI module **reassigns and wraps** core functions (`nav` / `render` /
  `libraryCards` / `detail` / `closeModal` / `fresh` / `validate` / `addBtn`) and inserts
  the 「拍照录题」 / 「举一反三」 pages with `NAV.splice(4,0,…)`.
- **Order matters**: the AI segment must stay after the core segment (`addBtn` is declared
  with `let`; the AI segment assigns to it). Moving it earlier crashes the app.
- Line numbers drift after edits; search by content/function name, not by line.
- No framework, no components. **All interactions go through `data-action="xxx"`**,
  dispatched by two delegated listeners (one core, one AI). A new button = a new
  `data-action` branch. Do not add `onclick` — the existing 4 are whitelisted in
  `tests/test_static.py`.
- Core `render()` uses a `({home:…,library:…})[view]()` map; the AI segment wraps `render`
  to own its pages. New pages must update that map too.

## 3. State and persistence (front end)

- **Study data lives only in SQLite** via the FastAPI backend; the browser stores none. On
  load, legacy localStorage keys are cleaned once (the only allowed localStorage touch).
- AI config (provider / models / endpoint, **no key**) is also stored in SQLite and travels
  with `state.aiConfig`.
- **The only write path is `commit(fn)`**: clone state → `validate()` → swap in-memory
  `state` → `POST /api/ops` with id-level diffs. A validation failure throws a Chinese
  error and **keeps the old state**; a sync failure shows a warning and retries on the next
  commit. Assigning `state.xxx` directly neither validates nor persists.
- Changing the data shape requires updating `validate()` in lockstep (the AI segment adds
  `validateAISets` / `aiConfig` checks). Backup import also runs `validate()`; malformed
  files are rejected.
- Images are stored as data URLs in SQLite. `/api/state` returns `initialized` to
  distinguish a fresh database from an intentionally emptied one (so sample questions are
  not re-seeded after a clear).
- Opened via `file://` there is no `api-token`: the page is a read-only preview, `commit()`
  refuses immediately and nothing is written to any browser storage. AI features report an
  error telling the user to open via the local service. The former `transport` concept is
  fully removed (frontend `AI_DEFAULT`, backend `ai_config.transport` column, UI dropdown);
  `aiChat` always targets `/api/chat`.

## 4. Local service (FastAPI)

- `src/shiguang_study/webapp/`: `GET /` injects `meta[name="api-token"]`; every `/api/*`
  route except `/api/health` requires `Authorization: Bearer`. Data lands in SQLite.
  - `GET /api/state` — full state incl. `initialized` / `aiConfig`
  - `POST /api/ops` — applies batched incremental ops
  - `POST /api/import` — whole-database replacement
  - `POST /api/ai-key` / `DELETE /api/ai-key/{provider}` — store / remove a key
  - `POST /api/chat` — provider forwarding with the stored key
  - `GET /api/status` — per-provider `hasKey` + masked preview only
- Configuration is environment-only: `SHIGUANG_TOKEN`, `SHIGUANG_DATA_DIR`, `SHIGUANG_DB`,
  `SHIGUANG_HOST`, `SHIGUANG_PORT`, `SHIGUANG_ENV_FILE`. Default bind is `127.0.0.1:4312`;
  there is no public account system, so never expose it directly.
- `.env` lookup order: `SHIGUANG_ENV_FILE` → current directory → project root → data
  directory. Variables inside `.env` never override real environment variables.
- The app page is resolved by `config.app_path()`: `SHIGUANG_APP` may point at an absolute
  path, otherwise it walks up at most 6 levels from the package looking for
  `src/app/index.html`. Renaming the app file requires updating this too.

## 5. Key storage and security

- API keys live **only on the backend**: the page posts once to `POST /api/ai-key` → the
  service encrypts with `SHIGUANG_SECRET` from `.env` (AES-256-GCM, `keys.py`) → ciphertext
  is stored in SQLite `ai_keys` keyed by owner + provider → decrypted per request.
- The browser never sees plaintext: the page keeps only `aiCreds` (`hasKey` + masked
  preview). `/api/status` returns `hasKey` and `preview` (first 4 + `****` + last 4; keys
  of ≤12 characters are fully masked). `/api/chat` **rejects** an `apiKey` in the request
  body.
- The masked preview is **generated by the backend at runtime**; the HTML source and any
  backup contain no key material at all (not even masked).
- The preview is shown directly in the input's value (`aiCredField()` template): on submit,
  a value equal to the preview counts as "untouched" and is skipped; focusing the field
  selects it (`focusin` delegate on `.ai-keyinput`).
- **Panel partial refresh goes through `aiRefreshCredUI()`**: `aiRefresh()` only rebuilds
  the two AI pages, so on 「偏好与数据」 the panel state (e.g. the key hint) must be
  replaced locally via `aiRefreshCredUI()`.
- `SHIGUANG_SECRET` lives in `.env` (**not committed**, gitignored, owner-only file
  permissions, generated on first use). Losing it makes stored ciphertext unreadable —
  the only recovery is re-entering keys.
- **Never** write a plaintext key into HTML, backup JSON, AI config, docs or any artifact;
  logs and error messages must not leak it either.
- Clearing study data (reset / backup import) **keeps** keys — they are service
  credentials, not study data.

## 6. Provider forwarding and allowlist (`llm.py`)

- Qwen endpoints must match `*.aliyuncs.com` with a path ending in `/compatible-mode/v1`;
  DeepSeek must be `api.deepseek.com` (path empty / `/` / `/v1`). Checked twice: frontend
  `aiValidateConfig` and backend `endpoint_provider`.
- Only allowlisted request fields are forwarded. Qwen receives `enable_thinking:false`;
  DeepSeek receives `thinking:{type:'disabled'}`. The two providers' keys **never fall
  back to each other**. Request cap 25 MB, response cap 3 MB. Redirects carrying the key
  are refused.

## 7. AI-flow invariants (do not break)

- Variation practice = **two independent API calls**: `aiGenerate` produces 3 questions,
  then `aiReview` independently checks them; only `verified:true` may be persisted. Any
  failure / timeout / truncation / duplicate → **never save a half set**. Do not merge the
  two calls.
- Recognition follows the same shape: a proofreading draft first; questions enter the
  library only after the user ticks them and `aiSaveDrafts` runs; already-stored questions
  are never re-inserted or re-scored.
- Always `response_format:{type:'json_object'}` with `stream:false`; treat
  `finish_reason === 'length'` as truncation and error out.
- Cancelling only stops waiting — a request already in flight may still be billed.
- Practice is self-assessed (还不会 / 有点模糊 / 掌握了); nothing is auto-graded.

## 8. Hard limits

| Item | Limit |
|---|---|
| Photo | ≤12 MB, JPG/PNG/WebP only |
| Material batch | ≤12 images/pages |
| PDF | ≤30 MB, ≤200 pages |
| Pages per selection | ≤8 |
| Image compression | long edge ≤2100 px, data URL ≤2.1 MB |
| Question / answer text | ≤10000 characters |
| AI practice sets kept | 30 most recent |

## 9. Review & points rules (do not change by accident)

Stage 0–5. 「还不会」 → next day; 「有点模糊」 → +2 days; 「掌握了」 → `[1,3,7,14,30]` days
(stage 5 stays at 30). 3 consecutive 「掌握了」 = mastered (stage ≥ 3). New question +3;
first review of a question per day +5 (0 afterwards); focus +1/minute capped at +30 per
session; reward redemption does **not** reduce cumulative growth. (User-facing wording:
user-guide.md.)

## 10. PDF.js runtime

Core / worker are decoded at runtime from the inline base64 blocks (requires
`Promise.withResolvers` → recent Chrome / Edge / Safari); `pdfjs-resources` is a
gzip+base64 JSON decoded with `DecompressionStream('gzip')`. Editing these blocks equals
recompiling PDF.js — forbidden.

## 11. Verification

```bash
uv run pytest                                 # 104 tests
uv run python -m py_compile src/shiguang_study/*.py src/shiguang_study/webapp/*.py
```

The suites run under **pytest** (dev dependency group; loops that used to be `subTest`
checks are expanded into parametrized cases). `tests/conftest.py` puts `src/` on
`sys.path`. What each suite guards:

- `tests/test_static.py` — single inline `<script>`; PDF.js resource blocks present; AI
  segment after core; `onclick` whitelist; **no `localStorage.getItem`/`setItem`** (one
  legacy cleanup allowed); AI config persisted via `state.aiConfig`; no `sk-` key
  literals; both providers' thinking parameters; generation must pass review; **no
  `transport`, no `/chat/completions`, no `aiKey`/`aiDeepseekKey` page variables**;
  `/api/ai-key` is called; `pyproject.toml` declares the script entry; `uv.lock` exists.
- `tests/test_keys.py` — `.env` parsing, secret generation and writing, encrypt/decrypt
  round-trips, rejection of wrong key / tampered / corrupted ciphertext, **masking rules**.
- `tests/test_llm.py` — endpoint allowlist, request-body field trimming, per-provider
  thinking parameters, key masking, oversized responses, network failures.
- `tests/test_webapp_http.py` — Bearer auth, state/ops/import round-trips, aiConfig, key
  encryption at rest and deletion, **status returns only the masked preview**, chat uses
  the stored key, body-injected keys rejected, millisecond timer timestamps.
- `tests/test_webapp_isolation.py` — cross-owner writes and transaction rollback, key
  ownership, clearing data keeps keys, legacy `transport` column migration.
- `tests/test_webapp_main.py` — the launcher opens the browser only after uvicorn owns
  its listening socket, and the default (no `--open`) never opens one.

Beyond static checks, verify manually in a browser: `uv run shiguang-study` for data sync
and AI features; double-click `src/app/index.html` for the read-only notice.
`docs/acceptance-report.md` PASS lists serve as regression checklists. After changing
behaviour, update that report together with `user-guide.md` / `ai-guide.md`.

## 12. Conventions

- UI text, code comments inside the app, and error messages are Chinese; identifiers are
  English; repo docs are English.
- Keep the existing compact single-line style; no build tools, frameworks or npm.
- User-visible behaviour change → update `user-guide.md` / `ai-guide.md` in the same pass.
