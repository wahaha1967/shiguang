# Shiguang Study Room · 拾光自习室

An offline-first, single-file web workbench for students: keep a wrong-answer book, review it
on a spaced schedule, practise randomly, time focus sessions — and optionally let a vision
model turn photos and exam PDFs into reviewable questions.

The application UI is in Chinese (it targets Chinese middle and primary school students).
The project layout, code comments for the service layer, and all documentation are in English.

## Layout

```
.
├── pyproject.toml             uv project definition (uv_build backend, console script)
├── uv.lock                    locked environment - commit it
├── .python-version            pinned interpreter for uv
├── src/
│   ├── app/index.html         the product: UI + core logic + AI logic + embedded PDF.js
│   └── shiguang_study/        the local AI bridge package
│       ├── __init__.py
│       ├── __main__.py
│       └── bridge.py
├── scripts/
│   ├── build.py               assembles the shipping bundle into dist/
│   ├── launch.sh              POSIX launcher (macOS / Linux)
│   └── launch.bat             Windows launcher
├── tests/                     unittest suites (no network required)
├── docs/                      user guide, AI guide, acceptance report
├── licenses/                  PDF.js (Apache-2.0) and font/decoder licences
└── dist/                      generated bundle - this is what you ship
```

`src/app/index.html` is the single source of truth for the app. `scripts/build.py` copies it,
the bridge, the launchers and the licences into `dist/`, which is the folder a user receives
and double-clicks.

## Development environment (uv)

The Python side is managed with [uv](https://docs.astral.sh/uv/). The bridge itself has **no
third-party dependencies** - it only uses the standard library - but uv provides the
interpreter, the virtual environment, the lock file and the console entry point.

```bash
uv sync                                  # create .venv and install the project
uv run shiguang-study                    # start the workbench (console script)
uv run python -m shiguang_study          # same thing, module form
uv run python -m unittest discover -s tests
uv run python scripts/build.py           # rebuild dist/
uv run python scripts/build.py --check   # verify dist/ matches the sources
uv add <package>                         # add a dependency (never pip install)
uv lock                                   # refresh uv.lock after manual edits
```

`uv.lock` and `.python-version` are committed; `.venv/` is not.

## Quick start

```bash
# 1. build the shipping bundle
uv run python scripts/build.py

# 2. run it (opens http://127.0.0.1:4312/ automatically)
uv run shiguang-study
#   or: ./scripts/launch.sh        (source tree)
#   or: double-click dist/launch.command / dist/launch.bat
```

Without the server, everything except the AI features works by opening `dist/study-room.html`
directly in a browser - no install, no network, no Python.

## Commands

| Command | Purpose |
|---|---|
| `uv sync` | Create/update `.venv` from `uv.lock` |
| `uv run shiguang-study` | Start the local service (console script) |
| `uv run python -m shiguang_study` | Start the local service (module form) |
| `uv run python scripts/build.py` | Rebuild `dist/` from the sources |
| `uv run python scripts/build.py --check` | Verify `dist/` matches the sources (exit 1 if stale) |
| `uv run python -m unittest discover -s tests` | Run the test suites |
| `uv run shiguang-study --port 5000 --no-open` | Start the local service on another port |
| `DASHSCOPE_API_KEY=... uv run shiguang-study` | Keep the Qwen key on the server side |
| `python3 dist/bridge.py` | Run the shipped bundle without uv (plain stdlib Python) |

## How it works

- **One HTML file.** The whole interface, its styles, the core app and the AI module live in
  a single `<script>` block; PDF.js and its Chinese character maps, fonts and decoders are
  embedded as base64/gzip resources. There is no build step for the app itself and no CDN
  dependency at runtime.
- **Local storage is the database.** Learning data lives under `shiguang-study-v1`; AI
  provider settings under `shiguang-ai-config-v1`. Every write goes through `commit()`,
  which validates the whole state before storing it and keeps the previous data if
  validation fails.
- **Keys never touch disk.** API keys live in the page's memory, or in the service's
  environment. `/api/status` only reports whether a key is configured, never its value.
- **Two transports.** Opened through the local service the app talks to `/api/chat` on the
  bridge; opened directly it talks to the provider from the browser. The bridge only accepts
  the official Qwen and DeepSeek endpoints, refuses redirects that carry the key, and never
  falls back from one provider's credential to the other's.
- **AI output is verified before it is stored.** Variant practice makes two independent model
  calls — one to generate, one to review — and saves nothing if the review fails.

## Documentation

- [docs/user-guide.md](docs/user-guide.md) — how to use the workbench
- [docs/ai-guide.md](docs/ai-guide.md) — photo/PDF capture, variation practice, provider setup
- [docs/acceptance-report.md](docs/acceptance-report.md) — what has been verified, and what has not

## Requirements

- A modern browser (Chrome / Edge / Safari) for the PDF features.
- Python 3.9+ for the local service — standard library only, nothing to install.
- [uv](https://docs.astral.sh/uv/) if you want to work on the source tree; the shipped bundle
  in `dist/` runs on plain Python and never needs uv.

## Licences

PDF.js is a Mozilla project under the Apache License 2.0; the bundled fonts and decoders have
their own notices. Both are preserved in `licenses/` and inside the shipped bundle.
