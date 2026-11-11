# Shiguang Study Room · 拾光自习室

A local-first, single-file web workbench for students: keep a wrong-answer book, review it
on a spaced schedule, practise randomly, time focus sessions — and optionally let a vision
model turn photos and exam PDFs into reviewable questions. A FastAPI backend stores the
data in SQLite.

The application UI is in Chinese (it targets Chinese middle and primary school students).
The project layout, code comments for the service layer, and all documentation are in English.

## Layout

```
.
├── pyproject.toml             uv project definition (uv_build backend, console script)
├── uv.lock                    locked environment - commit it
├── .python-version            pinned interpreter for uv
├── .env                       local secrets (SHIGUANG_SECRET) - not committed
├── src/
│   ├── app/index.html         the product: UI + core logic + AI logic + embedded PDF.js
│   └── shiguang_study/
│       ├── __init__.py
│       ├── __main__.py        console entry point (starts FastAPI)
│       ├── webapp/            FastAPI + SQLAlchemy + SQLite backend
│       ├── keys.py            .env handling and AES-256-GCM encryption of the API keys
│       └── llm.py             Qwen / DeepSeek forwarding (stdlib HTTP)
├── tests/                     pytest suites (no network required)
├── docs/                      architecture guide, user guide, AI guide, acceptance report
└── licenses/                  PDF.js (Apache-2.0) and font/decoder licences
```

`src/app/index.html` is the single source of truth for the app. `uv run shiguang-study` serves
it and stores study data in SQLite. Opening the HTML directly is a read-only preview.

## Development environment (uv)

The Python side is managed with [uv](https://docs.astral.sh/uv/). The backend is FastAPI +
SQLAlchemy + uvicorn; SQLite needs no separate server.

```bash
uv sync                                  # create .venv and install the project
uv run shiguang-study                    # start the FastAPI backend (console script)
uv run python -m shiguang_study          # same thing, module form
uv run pytest                              # run the test suites
uv add <package>                         # add a dependency (never pip install)
uv lock                                   # refresh uv.lock after manual edits
```

`uv.lock` and `.python-version` are committed; `.venv/` and `.env/` are not.

## Quick start

```bash
# 1. install dependencies
uv sync

# 2. start the FastAPI backend, then open http://127.0.0.1:4312/ yourself
uv run shiguang-study

# or let it open the browser once the server is ready
uv run shiguang-study --open
```

Opening `src/app/index.html` directly is a read-only preview: it shows the sample data but
cannot save. The FastAPI backend keeps the data in SQLite.

## Commands

| Command | Purpose |
|---|---|
| `uv sync` | Create/update `.venv` from `uv.lock` |
| `uv run shiguang-study` | Start the FastAPI backend (console script) |
| `uv run python -m shiguang_study` | Start the FastAPI backend (module form) |
| `uv run pytest` | Run the test suites |
| `uv run shiguang-study --port 5000` | Start the local service on another port |
| `uv run shiguang-study --open` | Open the app in a browser once the server is ready |

## How it works

- **One HTML file.** The whole interface, its styles, the core app and the AI module live in
  a single `<script>` block; PDF.js and its Chinese character maps, fonts and decoders are
  embedded as base64/gzip resources. There is no build step for the app itself and no CDN
  dependency at runtime.
- **SQLite is the only database.** The FastAPI backend stores learning data and the
  non-secret AI settings (provider, models, endpoints) in SQLite. The browser keeps no
  learning data; legacy browser keys are removed once on load. Every write goes through
  `commit()`, which validates the whole state before sending the changed rows to `/api/ops`;
  a failed sync is reported and retried on the next action.
- **The HTML alone is a read-only preview.** Opened directly (`file://`), the page shows the
  sample data but refuses to save. Run the FastAPI backend to keep data.
- **The provider key stays on the backend.** The page sends the key once to
  `POST /api/ai-key`; the service encrypts it with `SHIGUANG_SECRET` from `.env`
  (AES-256-GCM) and stores only the ciphertext in SQLite. Every request decrypts it again.
  `/api/status` only reports whether a key is configured, never its value, and the key is
  never part of the study backup.
- **The browser never calls the provider.** Every model request goes through `/api/chat` on
  the backend. It only accepts the official Qwen and DeepSeek endpoints, refuses redirects
  that carry the key, and never falls back from one provider's credential to the other's.
- **AI output is verified before it is stored.** Variant practice makes two independent model
  calls — one to generate, one to review — and saves nothing if the review fails.

## Documentation

- [docs/architecture.md](docs/architecture.md) — developer reference: anatomy, persistence, keys, forwarding, limits, tests
- [docs/user-guide.md](docs/user-guide.md) — how to use the workbench
- [docs/ai-guide.md](docs/ai-guide.md) — photo/PDF capture, variation practice, provider setup
- [docs/acceptance-report.md](docs/acceptance-report.md) — what has been verified, and what has not

## Requirements

- A modern browser (Chrome / Edge / Safari) for the PDF features.
- Python 3.9+ and [uv](https://docs.astral.sh/uv/) for the FastAPI backend.

## Licences

PDF.js is a Mozilla project under the Apache License 2.0; the bundled fonts and decoders have
their own notices. Both are preserved in `licenses/` and inside the application HTML.
