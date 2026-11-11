# Behaviour parity with the Python backend

The Go backend reproduces the FastAPI contract observed by `web/index.html`.
Golden fixtures in `../fixtures` were generated from the frozen Python release
and are replayed by `internal/ops/fixtures_test.go`, `internal/llm/llm_test.go`
and `internal/secret/secret_test.go`.

The differences below are deliberate and do not affect requests the frontend
actually sends.

## JSON parsing

- **Malformed JSON** returns `400 {"detail":"请求格式不正确"}`. FastAPI/Pydantic
  returns `422` with a validation array. The page only reads `detail`, so the
  message stays readable. A top-level `null` body is rejected the same way.
- **Trailing bytes** after the JSON object are rejected with `400`; FastAPI
  ignores the remainder.
- Numbers are decoded with `json.Decoder.UseNumber()` so integers are
  distinguishable from floats, matching Python's `isinstance(value, int)`.
- Text limits count Unicode code points (`utf8.RuneCountInString`), matching
  Python `len()`.

## Operation validation

- `logs.qid` and `logs.grade` accept `null` or a string and reject other types.
  Python writes values without type checks. This is a defensive narrowing; the
  page sends strings or omits the fields.
- A non-object `row` (including `false`, `0` and `""`) is rejected. Python's
  `op.get('row') or {}` treated those values as an empty row; only malformed
  input is affected.
- The Go build rejects an outbound chat request larger than 25 MB with `502`
  and `请求内容过大，请减少题量`. Python defines the same `MAX_REQUEST`
  constant but never enforces it; the page stays far below the limit.

## State serialization

- Integer-valued floats may be encoded without a decimal part (Go writes `60`,
  Python writes `60.0`). JavaScript treats both as the same number, and the
  fixture comparator normalizes numeric values for the same reason.
- `ai_config` columns that are SQL `NULL` are returned as JSON `null`; columns
  keep their Python names (`deepseekBase`, `deepseekText`, …).
- `timer.end` and `logs.qid`/`grade`/`minutes` are omitted when absent, as in
  Python.
- `GET /` and `/index.html` inject the token meta tag before **every**
  `</head>`, matching Python's `str.replace` without a count.

## Desktop-only additions

- `--serve`, `--port`, and `--open` flags, and `SHIGUANG_DEV=1`.
- A per-user named mutex (`Local\ShiguangStudyRoom`). A second launch shows a
  Chinese message box and exits.
- GUI mode logs to `study.log` in the data directory (serve mode logs to
  stderr). Logs never contain plaintext keys.
- `.env` lookup is `SHIGUANG_ENV_FILE`, then the data directory, then the
  executable directory (the last is read-only and trusted only for
  `SHIGUANG_SECRET`; see the hardening section). Python searched the working
  directory and the source tree, so point `SHIGUANG_ENV_FILE` at an old `.env`
  to reuse its keys.
- On Windows, file mode `0600` for `.env`/`token` is best effort; the files live
  inside the user profile.

## Deliberate hardening over the Python backend

These go beyond parity. They close holes that only matter once the service runs
as a desktop app on a machine that also browses the web.

- **Host allowlist.** Every request must address the loopback interface
  (`127.0.0.1`, `localhost`, `::1`); anything else gets `403 只接受本机访问`.
  `GET /` is unauthenticated and injects the API token, so without this a
  website that DNS-rebinds its own domain to `127.0.0.1` could read the token
  same-origin and then read or write all data. The frontend always uses a
  relative URL against `127.0.0.1:<port>`, so it is unaffected.
- **Untrusted exe-adjacent `.env`.** A `.env` found next to the executable may
  only supply `SHIGUANG_SECRET`; every other key (`SHIGUANG_TOKEN`,
  `SHIGUANG_DATA_DIR`, …) is ignored. Whoever can drop a file beside the binary
  must not be able to redirect the data directory or the token. The data-dir
  `.env` and an explicit `SHIGUANG_ENV_FILE` stay fully trusted.
- **Request body cap.** `http.MaxBytesReader` bounds each request body
  (`maxRequestBody`, default 256 MB); an oversized body fails decoding and
  returns `400 请求格式不正确`. This is separate from the 25 MB *outbound* chat
  cap noted above.
- **Server timeouts.** `ReadHeaderTimeout` 5s, `ReadTimeout` 120s,
  `WriteTimeout` 150s (above the 100s chat upstream), `IdleTimeout` 120s, so a
  stalled client cannot hold a connection open.
- **Cache headers.** JSON responses send `Cache-Control: no-store`. The page
  sends `Cache-Control: no-cache` with a content-hash `ETag` and honours
  `If-None-Match` with `304`, so the token-bearing HTML is never cached but is
  not re-sent when unchanged.

## Unchanged security behaviour

- Bearer auth, constant-time comparison, and the exact 401/403 Chinese details.
- AES-256-GCM key format and preview masking, interoperable both ways with the
  Python backend.
- Endpoint allowlist, no redirects, payload whitelist, thinking flags,
  100-second timeout, 3 MB response cap, and key redaction
  (`[密钥已隐藏]`) in upstream errors.
- Owner id is the first 16 hex characters of `sha256(token)`, so old data is
  visible under the same token.
