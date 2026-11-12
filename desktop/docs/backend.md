# Backend behaviour & security

The Go backend serves the single-file UI (`web/index.html`) over a loopback HTTP
service. This document records the request/response contract, the validation
rules, and the security hardening the desktop build applies. The golden fixtures
in `../fixtures` are frozen samples replayed by the Go test suite.

## JSON parsing

- **Malformed JSON** returns `400 {"detail":"请求格式不正确"}`. A top-level `null`
  body is rejected the same way. The page only reads `detail`, so the message
  stays readable.
- **Trailing bytes** after the JSON object are rejected with `400`.
- Numbers are decoded with `json.Decoder.UseNumber()` so integers stay
  distinguishable from floats.
- Text limits count Unicode code points (`utf8.RuneCountInString`).

## Operation validation

- `logs.qid` and `logs.grade` accept `null` or a string and reject other types.
- A non-object `row` (including `false`, `0` and `""`) is rejected.
- An outbound chat request larger than 25 MB is rejected with `502` and
  `请求内容过大，请减少题量`.

## State serialization

- Integer-valued floats may be encoded without a decimal part (`60` rather than
  `60.0`). JavaScript treats both as the same number, and the fixture comparator
  normalizes numeric values for the same reason.
- `ai_config` columns that are SQL `NULL` are returned as JSON `null`; the JSON
  field names are `deepseekBase`, `deepseekText`, and so on.
- `timer.end` and `logs.qid`/`grade`/`minutes` are omitted when absent.
- `GET /` and `/index.html` inject the token and build-info meta tags before
  **every** `</head>`.

## Build info

- `internal/buildinfo` holds `Version` (the product version; bump it together
  with `build/windows/info.json` and `build/windows/app.manifest`), `Commit`,
  and `BuiltAt` (Unix seconds). `Commit`
  and `BuiltAt` are stamped by `wails3 task windows:build` through
  `-ldflags -X …buildinfo.Commit=<git short hash> -X …buildinfo.BuiltAt=<unix>`;
  a plain `go run` leaves them empty.
- The page receives `<meta name="app-build" content="版本|提交|构建日期">`, where
  the date is rendered as `2006-01-02 上午/下午` in local time (noon starts 下午).
  The content is HTML-escaped. The 关于 dialog reads this meta; with no meta
  (`file://` preview) or empty fields it falls back to 开发构建 / 离线预览.

## Desktop behaviour

- `--serve`, `--port`, and `--open` flags, and `SHIGUANG_DEV=1`.
- A per-user named mutex (`Local\ShiguangStudyRoom`). A second launch shows a
  Chinese message box and exits.
- GUI mode logs to `study.log` in the data directory (serve mode logs to stderr).
  Logs never contain plaintext keys.
- `.env` lookup is `SHIGUANG_ENV_FILE`, then the data directory, then the
  executable directory (the last is trusted only for `SHIGUANG_SECRET`; see the
  hardening section).
- On Windows, file mode `0600` for `.env`/`token` is best effort; the files live
  inside the user profile.

## Security hardening

These close holes that matter once the service runs as a desktop app on a machine
that also browses the web.

- **Host allowlist.** Every request must address the loopback interface
  (`127.0.0.1`, `localhost`, `::1`); anything else gets `403 只接受本机访问`.
  `GET /` is unauthenticated and injects the API token, so without this a website
  that DNS-rebinds its own domain to `127.0.0.1` could read the token same-origin
  and then read or write all data. The frontend always uses a relative URL against
  `127.0.0.1:<port>`, so it is unaffected.
- **Untrusted exe-adjacent `.env`.** A `.env` found next to the executable may only
  supply `SHIGUANG_SECRET`; every other key (`SHIGUANG_TOKEN`, `SHIGUANG_DATA_DIR`,
  …) is ignored. Whoever can drop a file beside the binary must not be able to
  redirect the data directory or the token. The data-dir `.env` and an explicit
  `SHIGUANG_ENV_FILE` stay fully trusted.
- **Request body cap.** `http.MaxBytesReader` bounds each request body
  (`maxRequestBody`, default 256 MB); an oversized body fails decoding and returns
  `400 请求格式不正确`. This is separate from the 25 MB *outbound* chat cap above.
- **Server timeouts.** `ReadHeaderTimeout` 5s, `ReadTimeout` 120s, `WriteTimeout`
  150s (above the 100s chat upstream), `IdleTimeout` 120s, so a stalled client
  cannot hold a connection open.
- **Cache headers.** JSON responses send `Cache-Control: no-store`. The page sends
  `Cache-Control: no-cache` with a content-hash `ETag` and honours `If-None-Match`
  with `304`, so the token-bearing HTML is never cached but is not re-sent when
  unchanged.

## Security invariants

- Bearer auth, constant-time comparison, and the exact 401/403 Chinese details.
- AES-256-GCM key format (`base64(nonce || sealed)`, keyed by
  `sha256(SHIGUANG_SECRET)`) and preview masking.
- Endpoint allowlist, no redirects, payload whitelist, thinking flags, 100-second
  timeout, 3 MB response cap, and key redaction (`[密钥已隐藏]`) in upstream errors.
- Owner id is the first 16 hex characters of `sha256(token)`, scoping one token's
  data space.