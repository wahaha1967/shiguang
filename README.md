# Shiguang Study Room · 拾光自习室

**Turn wrong answers into mastered skills.** A local-first study workbench for students that
ships as a single Windows `.exe` — no cloud account, no extra runtime to install, no browser tab
to babysit.

[中文版](README_zh.md)

![Windows](https://img.shields.io/badge/Windows-10%2F11%20x64-0078D6?logo=windows&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)
![Wails](https://img.shields.io/badge/Wails-v3.1.0-DF3A01)
![SQLite](https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite&logoColor=white)

拾光自习室 helps students keep a wrong-answer book, review it on a spaced-repetition schedule,
practise randomly, and time focus sessions. Optionally, a vision model turns photos and exam
PDFs into reviewable questions, and *variation practice* (举一反三) generates verified
look-alike problems around the same knowledge point.

Interface and backend are compiled into one executable; study data lives in a local SQLite
database on your machine. Double-click and study.

> The application UI is in Chinese (it targets Chinese middle- and primary-school students).
> This file is the English reference; [README_zh.md](README_zh.md) is its Chinese counterpart.

## Features

**Learn from your mistakes**

- Wrong-answer book across nine subjects — 语文 · 数学 · 英语 · 物理 · 化学 · 生物 · 政治 ·
  历史 · 地理 — each entry holding the question, reference answer, what went wrong and the
  knowledge point, with an optional photo.
- Spaced repetition: rate yourself 还不会 / 有点模糊 / 掌握了 and the next review returns in
  1 / 3 / 7 / 14 / 30 days. Three "mastered" ratings in a row retire a question to 已掌握.
- Random practice, a statistics page, and a recycle bin so nothing is lost by accident.

**Stay motivated**

- A focus timer that awards points per minute, capped per session.
- Growth points — +3 per new question, +5 for the first review of the day, +1 per focused
  minute — and self-defined rewards you can redeem. Spending points never lowers your
  cumulative growth value.

**Capture questions fast (optional AI)**

- Photo capture: snap or upload JPG/PNG/WebP and a vision model transcribes it into reviewable
  questions; images are compressed locally before upload.
- PDF capture: text or scanned exam PDFs are rendered locally with embedded PDF.js, and only
  the pages you pick are recognised — nothing is downloaded from a CDN.
- Variation practice (举一反三): three similar questions are generated around the same
  knowledge point, then a second, independent model call verifies wording, answer and
  explanation before anything is saved.
- Works with Tongyi Qwen (default) or DeepSeek. The two providers keep separate keys and
  settings; switching never mixes them.

**Your data, your machine**

- One-click JSON backup (questions, images, review logs, rewards, preferences, AI settings) and
  CSV export; import to restore on another machine. API keys are never part of the backup.
- Print or save any view as PDF.
- Local-first: no account, no cloud, a loopback-only service.

## Get started

### Just want to use it?

1. Build the app: `cd desktop && wails3 task package` produces
   `desktop/dist/拾光自习室/拾光自习室.exe` (plus a `windows-x64` zip). See **Build from source**
   below for prerequisites.
2. Double-click `拾光自习室.exe` and keep its window open while you study. If Windows shows an
   "unknown publisher" prompt, choose *More info → Run anyway*.
3. Work through the 18 sample questions to see how everything fits, then set your name, daily
   goal, focus length and theme in 「偏好与数据」 (*Preferences & Data*).
4. Record your own wrong answers, review them daily, and export a JSON backup now and then.

All you need is Windows 10/11 x64 and the WebView2 runtime (bundled with Windows 11; most
Windows 10 machines already have it). Full end-user instructions live in
[`desktop/使用说明.txt`](desktop/使用说明.txt) and [docs/user-guide.md](docs/user-guide.md).

### Want the AI features?

Photo/PDF recognition and variation practice need a network connection and a Qwen or DeepSeek
API key, entered once in the AI settings and then kept encrypted by the local service. See
[docs/ai-guide.md](docs/ai-guide.md).

> Opening `src/app/index.html` directly in a browser (`file://`) is a **read-only preview**: it
> shows the sample data but cannot save. Run the desktop app to keep your data.

## Privacy & security

Local-first is the point: your study data and your provider key stay on your machine.

- No account, no cloud — the service binds `127.0.0.1` only and is never exposed to a network.
- Provider API keys are stored only as ciphertext (AES-256-GCM); they never appear in the
  page, backups, logs or error messages.
- Model requests only go to official Qwen/DeepSeek HTTPS endpoints, and AI-generated questions
  must pass an independent verification call before anything is saved.

The full request/response contract, validation rules and hardening notes are in
[`desktop/docs/backend.md`](desktop/docs/backend.md).

## Data storage

All study data lives under the user profile:

```
%USERPROFILE%\.local\share\shiguang-study\
├── study.db      # SQLite database with all study data
├── token         # access token for the local service
└── .env          # secret used to encrypt provider keys
```

**Keep the `.env` file: without it the stored keys cannot be decrypted.**

## Build from source

To **run** the packaged app you only need Windows 10/11 x64 + WebView2 (above). To **build**:

- Go 1.26+ with `CGO_ENABLED=0`. An older local Go is fine: `GOTOOLCHAIN=auto` selects the
  toolchain declared in `desktop/go.mod` automatically inside `desktop/`.
- Wails v3.1.0 CLI (`wails3`) on `PATH`, installed once with
  `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0` (lands in `$(go env GOPATH)\bin`).

```bash
cd desktop
wails3 task package          # dist/拾光自习室/… plus a windows-x64 zip
wails3 task windows:build    # build/bin/shiguang-study.exe
wails3 task test             # go test ./...
wails3 task serve            # loopback HTTP only, for debugging (127.0.0.1:4312)
go run ./cmd/study           # run from source: desktop window, random loopback port
```

The development workflow — single-file UI, page sync and hash lock, repo conventions — is
documented in [agents.md](agents.md).

## Documentation

- [README_zh.md](README_zh.md) — Chinese version of this file
- [docs/user-guide.md](docs/user-guide.md) — how to use the workbench
- [docs/ai-guide.md](docs/ai-guide.md) — photo/PDF capture, variation practice, provider setup
- [desktop/docs/backend.md](desktop/docs/backend.md) — backend behaviour & security
- [desktop/使用说明.txt](desktop/使用说明.txt) — end-user Windows instructions

## Licences

PDF.js is a Mozilla project under the Apache License 2.0; the bundled fonts and decoders have
their own notices. Both are preserved in `licenses/` and inside the application HTML.