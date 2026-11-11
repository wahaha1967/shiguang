# Shiguang Study Room · 拾光自习室

A local-first study workbench for students: keep a wrong-answer book, review it on a spaced
schedule, practise randomly, time focus sessions — and optionally let a vision model turn
photos and exam PDFs into reviewable questions. It ships as a **single-file Windows desktop
app**: the whole UI is one HTML file and the backend is a Go loopback service embedded in the
same `.exe`. No Python, uv, venv, or Node runtime is needed to run it.

The application UI is in Chinese (it targets Chinese middle and primary school students).
Identifiers, code comments, and repository documentation are in English.

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
│   ├── Taskfile.yml        build / test / package tasks
│   └── 使用说明.txt         end-user Windows instructions
├── docs/                   user guide, AI guide, and the desktop port plan
└── licenses/               PDF.js (Apache-2.0) and font/decoder licences
```

`src/app/index.html` is the single source of truth for the UI. `desktop/web/index.html` is a
byte-for-byte copy that the Go binary embeds; `desktop/web/embed_test.go` hash-locks it, so
editing the page means updating `src/app/index.html`, re-copying it into `desktop/web/`, and
deliberately updating `indexSHA256` in that test. Opening the HTML directly (`file://`) is a
read-only preview: it shows the sample data but cannot save.

## Run and build

End users double-click `拾光自习室.exe` (see [`desktop/使用说明.txt`](desktop/使用说明.txt)).
Developers build and test from [`desktop/`](desktop/README.md):

```bash
cd desktop
wails3 task test              # go test ./...
wails3 task vet               # go vet ./...
wails3 task windows:build     # build/bin/shiguang-study.exe
wails3 task package           # dist/拾光自习室/… plus a windows-x64 zip
wails3 task serve             # loopback HTTP only, for debugging (127.0.0.1:4312)
```

Building needs Go 1.26+ with `CGO_ENABLED=0` and the Wails v3.1.0 CLI (`wails3`). A local Go
older than 1.26 still works: `GOTOOLCHAIN=auto` picks the required toolchain automatically
inside `desktop/`. Install the CLI once with
`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`; it lands in `$(go env GOPATH)\bin`,
which must be on `PATH`. Running the built app needs Windows 10/11 x64 and the WebView2 runtime
(bundled with Windows 11).

## How it works

- **One HTML file.** The interface, its styles, the core app, and the AI module live in a
  single `<script>` block; PDF.js and its Chinese character maps, fonts, and decoders are
  embedded as base64/gzip resources. There is no build step for the page itself and no CDN
  dependency at runtime.
- **SQLite is the only database.** The Go service stores learning data and the non-secret AI
  settings (provider, models, endpoints) in SQLite at
  `~/.local/share/shiguang-study/study.db`. The page keeps no learning data. Every write goes
  through `commit()`, which validates the whole state before sending the changed rows to
  `/api/ops`; a failed sync is reported and retried on the next action.
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

## Security

The service binds `127.0.0.1` only, rejects any non-loopback `Host` header (DNS-rebinding
protection), authenticates every API call with a per-install token, caps request bodies, sets
read/write/idle timeouts, and sends `no-store` on JSON. API keys exist only as ciphertext. The
full list lives in [`desktop/README.md`](desktop/README.md#security-invariants); intentional
differences from the original backend are in
[`desktop/docs/parity.md`](desktop/docs/parity.md).

## Documentation

- [desktop/README.md](desktop/README.md) — desktop build, layout, data compatibility, security
- [desktop/docs/parity.md](desktop/docs/parity.md) — behaviour parity and deliberate hardening
- [docs/user-guide.md](docs/user-guide.md) — how to use the workbench
- [docs/ai-guide.md](docs/ai-guide.md) — photo/PDF capture, variation practice, provider setup
- [docs/go-wails-windows-plan.md](docs/go-wails-windows-plan.md) — the desktop port plan (historical)

## Licences

PDF.js is a Mozilla project under the Apache License 2.0; the bundled fonts and decoders have
their own notices. Both are preserved in `licenses/` and inside the application HTML.
