# Shiguang Study Room · 拾光自习室

[中文版](README_zh.md)

A local-first study workbench for students: keep a wrong-answer book, review it on a spaced
schedule, practise randomly, time focus sessions — and optionally let a vision model turn
photos and exam PDFs into reviewable questions. It ships as a **single-file Windows desktop
app**: the whole UI is one HTML file and the backend is a Go loopback service embedded in the
same `.exe`. No Python, uv, venv, or Node runtime is needed to run it.

- Platform: Windows 10/11 x64
- GUI: Wails v3.1.0 + Microsoft Edge WebView2
- Storage: SQLite through `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0`)
- HTTP: standard library only, loopback interface only

The application UI is in Chinese (it targets Chinese middle and primary school students).
Identifiers and code comments are in English; this file is the English reference and
[README_zh.md](README_zh.md) is its Chinese counterpart.

## Repository layout

```
.
├── src/app/index.html      the product UI: interface + core logic + AI logic + embedded PDF.js
├── desktop/                the shippable Windows app (Go + Wails v3)
│   ├── web/index.html      byte-for-byte copy of src/app/index.html, embedded and hash-locked
│   ├── cmd/study/          entry point and command-line flags
│   ├── internal/           app, config, server, store, ops, secret, llm packages
│   ├── fixtures/           golden JSON replayed by the Go tests (frozen sample data)
│   ├── build/windows/      icon, manifest, and version resources for the .syso
│   ├── docs/parity.md      intentional behaviour differences from the original backend
│   ├── tools/syncpage/     copies the UI source into web/ and refreshes the hash lock
│   ├── Taskfile.yml        build / test / package tasks
│   └── 使用说明.txt         end-user Windows instructions
├── docs/                   user guide, AI guide, and the desktop port plan
└── licenses/               PDF.js (Apache-2.0) and font/decoder licences
```

`src/app/index.html` is the single source of truth for the UI. `desktop/web/index.html` is a
byte-for-byte copy that the Go binary embeds; `desktop/web/embed_test.go` hash-locks it, so
editing the page means updating `src/app/index.html`, re-copying it into `desktop/web/`, and
deliberately updating `indexSHA256` in that test. `wails3 task sync` performs the copy and the
hash update in one step. Opening the HTML directly (`file://`) is a
read-only preview: it shows the sample data but cannot save.

The Go packages under `desktop/internal/`:

| Package | Responsibility |
|---|---|
| `cmd/study` | entry point and flags |
| `internal/app` | Wails window, lifecycle, single-instance mutex |
| `internal/config` | data directory, token, owner id |
| `internal/server` | routes, bearer auth, token injection, error mapping |
| `internal/store` | SQLite connection, DDL, migrations |
| `internal/ops` | validation, transactions, state assembly |
| `internal/secret` | `.env` parsing, AES-256-GCM, preview |
| `internal/llm` | endpoint allowlist, no-redirect proxy, limits |

## Requirements

To **run**: Windows 10/11 x64 and the WebView2 runtime (bundled with Windows 11; most
Windows 10 machines already have it).

To **build**:

- Go 1.26+, with `CGO_ENABLED=0`. A local Go older than 1.26 is fine: `desktop/go.mod`
  declares `go 1.26.0` and `GOTOOLCHAIN=auto` selects the required toolchain automatically
  inside `desktop/`.
- Wails v3.1.0 CLI (`wails3`) on `PATH`, installed once with
  `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`. The binary lands in
  `$(go env GOPATH)\bin`, which must be on `PATH`.

## Build and test

```bash
cd desktop
wails3 task test              # go test ./...
wails3 task vet               # go vet ./...
wails3 task sync              # copy src/app/index.html into web/ and refresh the hash lock
wails3 task sync:check        # only verify the copy and the hash lock
wails3 task windows:build     # build/bin/shiguang-study.exe
wails3 task package           # dist/拾光自习室/… plus a windows-x64 zip
wails3 task serve             # loopback HTTP only, for debugging (127.0.0.1:4312)
```

`windows:build` generates the manifest/version `.syso` first, then builds with
`-H windowsgui` so no console window appears. The task runs the test suite and the
`sync:check` guard as dependencies, so a stale copy of the page fails the build.
End users double-click `拾光自习室.exe` (see [`desktop/使用说明.txt`](desktop/使用说明.txt)).

To run from source:

```bash
go run ./cmd/study            # desktop window, random loopback port
```

Command-line flags:

| Flag | Meaning |
|---|---|
| `--serve` | Start only the loopback HTTP service (debugging) |
| `--port N` | Fixed port; `0` (default) picks a free one |
| `--open` | With `--serve`, open the app in the default browser |

## Data and compatibility

The desktop build uses the same data directory, schema, token, owner key, and
encryption format as the Python backend:

```
%USERPROFILE%\.local\share\shiguang-study\
├── study.db      # SQLite database (11 tables, same schema)
├── token         # generated once, 32 random bytes, base64url
└── .env          # SHIGUANG_SECRET used to encrypt provider keys
```

An existing Python `study.db` opens directly. The legacy `ai_config.transport` column is
dropped on first open, exactly as before. Provider keys use the same AES-256-GCM format, so
they keep working as long as the same `SHIGUANG_SECRET` is used. Python searched for `.env`
from the working directory; the desktop build reads the data directory and the executable
directory. A `.env` next to the executable is treated as untrusted and may only supply
`SHIGUANG_SECRET`; other keys there are ignored. If the old `.env` lives elsewhere (for
example in the source tree), point `SHIGUANG_ENV_FILE` at it or copy it into the data
directory.

Overrides: `SHIGUANG_DATA_DIR`, `SHIGUANG_DB`, `SHIGUANG_TOKEN`, `SHIGUANG_ENV_FILE`,
`SHIGUANG_SECRET`, and `SHIGUANG_DEV=1` (serve `web/index.html` from disk instead of the
embedded copy).

## How it works

- **One HTML file.** The interface, its styles, the core app, and the AI module live in a
  single `<script>` block; PDF.js and its Chinese character maps, fonts, and decoders are
  embedded as base64/gzip resources. There is no build step for the page itself and no CDN
  dependency at runtime.
- **SQLite is the only database.** The Go service stores learning data and the non-secret AI
  settings (provider, models, endpoints) in SQLite at
  `%USERPROFILE%\.local\share\shiguang-study\study.db`. The page keeps no learning data.
  Every write goes through `commit()`, which validates the whole state before sending the
  changed rows to `/api/ops`; a failed sync is reported and retried on the next action.
- **The provider key stays in the backend.** The page sends the key once to `POST /api/ai-key`;
  the service encrypts it with `SHIGUANG_SECRET` (AES-256-GCM) and stores only the ciphertext
  in SQLite. `/api/status` reports whether a key is configured, never its value, and the key is
  never part of the study backup.
- **The page never calls the provider.** Every model request goes through `/api/chat`. It only
  accepts the official Qwen and DeepSeek HTTPS endpoints, refuses redirects that would carry
  the key, redacts the key from any upstream error, and never falls back from one provider's
  credential to the other's.
- **AI output is verified before it is stored.** Variant practice (举一反三) makes two
  independent model calls — one to generate, one to review — and saves nothing if the review
  fails.

## Security invariants

- Provider API keys exist only as ciphertext in SQLite. They never appear in the page,
  backups, logs, or error messages.
- `/api/chat` always uses the stored key and ignores any key in the request body.
- Only official Qwen/DeepSeek HTTPS endpoints are accepted, redirects are not followed, and
  only a whitelist of request fields is forwarded.
- The service binds `127.0.0.1` only. There is no account system and it must not be exposed
  to a network.
- Every request must address the loopback interface; a non-loopback `Host` (for example a
  DNS-rebinding attack) is rejected with `403` before any token is exposed.
- Request bodies are capped, the server sets read/write/idle timeouts, JSON responses send
  `Cache-Control: no-store`, and the token-bearing page sends `no-cache` with an `ETag`.
- A named mutex allows one instance per user session.

Intentional differences from the original backend are listed in
[`desktop/docs/parity.md`](desktop/docs/parity.md).

## Asset hash lock

`desktop/web/index.html` must not be edited directly. Any functional change belongs in
`src/app/index.html`; run `wails3 task sync` to copy it and to update `indexSHA256` and
`indexSize` in `desktop/web/embed_test.go` deliberately. `go test ./web/` and
`wails3 task sync:check` fail otherwise. `.gitattributes` normalizes every text file to LF
(`* text=auto eol=lf`) and marks the two page files as `-text`, so a checkout never rewrites
their bytes and the lock stays valid.

## Documentation

- [README_zh.md](README_zh.md) — Chinese version of this file
- [desktop/docs/parity.md](desktop/docs/parity.md) — behaviour parity and deliberate hardening
- [docs/user-guide.md](docs/user-guide.md) — how to use the workbench
- [docs/ai-guide.md](docs/ai-guide.md) — photo/PDF capture, variation practice, provider setup
- [docs/go-wails-windows-plan.md](docs/go-wails-windows-plan.md) — the desktop port plan (historical)

## Licences

PDF.js is a Mozilla project under the Apache License 2.0; the bundled fonts and decoders have
their own notices. Both are preserved in `licenses/` and inside the application HTML.
