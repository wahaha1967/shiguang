# Shiguang Study Room · AI Upgrade Acceptance Report

Date: 2026-10-22

> Note (2026-11-10): the `dist/` bundle and the standalone `bridge.py` have been retired.
> The app is now served by the FastAPI backend in `src/shiguang_study/webapp/`; study data
> lives in SQLite, not browser storage. This report is kept as a historical record.
>
> Note (2026-11-11): the provider key now lives in the backend. The page sends it once to
> `POST /api/ai-key`, the service encrypts it with `SHIGUANG_SECRET` from `.env`
> (AES-256-GCM) and stores only the ciphertext in SQLite; the browser keeps no key and
> never calls the provider itself. See the new section at the end of this report.

**Result:** 81 browser feature checks, 15 local proxy interface checks and 7 additional
independent-verification checks — 103 checks passed in total.

Browser coverage: rendering of 11 pages, compatibility with old data, JPEG/PNG upload
compression, camera permission denial, camera capture and track release, real rendering of
ordinary PDFs / Chinese-CMap PDFs / scanned-image PDFs, out-of-range page selection,
duplicate page selection, same-named files, fake PDFs and broken images, OCR proofreading,
blocking save when the answer is missing, batch save, image attachment, duplicate insertion,
re-recognition, three-question structure validation, duplicate-question validation, escaping
of AI text, independent answering and answer reveal, self-assessment de-duplication, practice
restore, variant insertion, interface authentication and rate-limit errors, malformed model
output, truncation, empty results, task cancellation, mobile horizontal overflow, plus
regression of the original wrong-answer CRUD / recycle bin / review / random practice / focus
timer.

Local interface coverage: local Host and Origin validation, session token, request size,
content type, static file allowlist, endpoint allowlist, request construction, rejecting
redirects that carry the key, key redaction in upstream errors, and environment keys never
being returned to the front end.

PDF processing actually ran in the browser. Camera testing used a simulated media stream and a
permission-denial case respectively; no real camera was used for shooting.

The protocol and flow were first verified against an isolated mock service. On 2026-10-22 a
real Bailian (Model Studio) integration was performed with a user-supplied key: qwen-plus
text connectivity, qwen3-vl-plus vision connectivity, photo recognition, two-page PDF
recognition, variant generation with independent verification, and answer refresh/save. The
student's wrong answer `x=6` in the photo sample was not taken as the correct answer; the
supplied correct answer was `x=4`; the two PDF pages had answers `x=4` and `x=6`. The first
real generation revealed an inconsistency between wording and explanation, which was fixed by
keeping the original question type and adding an independent review step; on retest the three
results `x=0`, `x=-1` and `x=4` were each verified by hand and by arithmetic. Passing
samples does not guarantee output quality for every subject or complex diagram question.

Static checks: the complete HTML embeds the PDF.js main program, the worker, Chinese
character maps, fonts and decoding resources; there is no runtime CDN dependency; no test key
is included; JavaScript and Python syntax checks pass.

## Browser and interface checks

| Result | Check |
|---|---|
| PASS | Page renders correctly: home |
| PASS | Page renders correctly: library |
| PASS | Page renders correctly: review |
| PASS | Page renders correctly: practice |
| PASS | Page renders correctly: capture |
| PASS | Page renders correctly: variations |
| PASS | Page renders correctly: calendar |
| PASS | Page renders correctly: stats |
| PASS | Page renders correctly: pet |
| PASS | Page renders correctly: focus |
| PASS | Page renders correctly: settings |
| PASS | Existing wrong answers and practice sets survive the upgrade |
| PASS | Question generation failure recovers without saving a half set: 401 |
| PASS | Question generation failure recovers without saving a half set: 429 |
| PASS | Question generation failure recovers without saving a half set: malformed |
| PASS | Question generation failure recovers without saving a half set: truncated |
| PASS | Question generation failure recovers without saving a half set: few |
| PASS | Question generation failure recovers without saving a half set: duplicate |
| PASS | Cancelling generation does not save a late result |
| PASS | AI text is shown as plain text; HTML is not executed |
| PASS | Switching questions quickly keeps the answer |
| PASS | Self-assessment cannot be recorded twice |
| PASS | API keys are not written to the config or the study backup |
| PASS | Old backups restore without AI fields |
| PASS | Incomplete AI practice backups are rejected |
| PASS | Overlong AI content is rejected, preventing silent truncation |
| PASS | A disguised PDF is rejected |
| PASS | A broken image produces a readable error |
| PASS | PDF page number 0 is rejected |
| PASS | PDF page numbers beyond the range are rejected |
| PASS | An invalid PDF page number reports the expected format |
| PASS | PDF page selection is de-duplicated and correctly sorted |
| PASS | At most 8 PDF pages per selection |
| PASS | A two-page PDF is actually rendered to local images |
| PASS | Reloading the same PDF page does not add material |
| PASS | Only ticked pages are recognised; with completion off the answer stays empty |
| PASS | A missing reference answer blocks saving |
| PASS | Re-recognition only replaces that page's unsaved draft |
| PASS | An empty recognition result fabricates no question and keeps the material for a retry |
| PASS | An OCR authentication failure keeps the page and the retry entry point |
| PASS | A retry after failure succeeds; AI-supplied answers are clearly marked |
| PASS | Clearing material asks for confirmation; saved questions are kept |
| PASS | Clearing PDF material releases the resources and closes the panel |
| PASS | Camera permission denial offers an upload alternative |
| PASS | A camera shot produces a local image |
| PASS | The camera track is released immediately after the shot |
| PASS | An ordinary PNG upload is compressed to JPEG |
| PASS | An oversized photo input is rejected |
| PASS | HEIC / unsupported photos give a conversion hint |
| PASS | The capture screen has no horizontal overflow on a phone |
| PASS | The variation screen has no horizontal overflow on a phone |
| PASS | No browser runtime errors or unhandled exceptions occurred |
| PASS | Real PDF rendering: ai-chinese.pdf |
| PASS | Real PDF rendering: ai-scanned.pdf |
| PASS | Chinese CMap, standard fonts and decoding resources are all embedded |
| PASS | A new PDF with the same name does not reuse the old page cache |
| PASS | Recognition does not write into the wrong-answer library by itself |
| PASS | After proofreading, wording, answer, reason and original image are saved |
| PASS | Each recognised question adds its 3 points exactly once |
| PASS | An already-saved recognised question cannot be inserted again |
| PASS | When the source question has an image, three complete variants can be generated |
| PASS | With an image, the vision model and the attached image are used, and the JSON is in non-thinking mode |
| PASS | Practice initially hides answers and explanations |
| PASS | Variant self-assessment and insertion are saved independently |
| PASS | An existing identical wording blocks a duplicate insertion |
| PASS | A refresh restores the variant answer and self-assessment |
| PASS | The original manual question entry still saves correctly |
| PASS | The original search still works |
| PASS | The original favourites still work |
| PASS | The question detail offers the variation-practice entry point |
| PASS | The original edit still works |
| PASS | The original delete moves the question to the recycle bin |
| PASS | The original recycle bin restore still works |
| PASS | The original preferences save correctly |
| PASS | The original review self-assessment and logging still work |
| PASS | The original random practice still starts |
| PASS | The original focus timer still starts |
| PASS | The original focus timer still pauses |
| PASS | The original focus abandon still works |
| PASS | A full backup of the new data validates and contains no keys |
| PASS | No runtime exceptions occurred in any interaction of this round |

## Additional independent-verification checks

| Result | Check |
|---|---|
| PASS | An independent verification is required after generation; the set is saved only on success |
| PASS | The verification state is clearly displayed |
| PASS | Generation and review are two independent interface calls |
| PASS | A failed verification does not save a partial set: reviewfalse |
| PASS | A failed verification does not save a partial set: reviewmalformed |
| PASS | The verification stage can be cancelled; a late result is not inserted |
| PASS | The verification mark is preserved in a full backup |

Real integration details are kept in the QA records. Keys were not written into the HTML,
source, test reports or the shared bundle — they only live in the local service's memory.

## 2026-10-22 addition: user-selectable Qwen / DeepSeek

30 provider-specific checks passed. The local proxy interface test now has 23 passing checks
(15 original plus 8 new). The original 103 results are kept above as the acceptance record of
the previous version; this round is a targeted regression for the provider change.

| Result | Check |
|---|---|
| PASS | The old Qwen configuration migrates automatically and Qwen remains the default provider |
| PASS | Only the Qwen configuration is shown by default |
| PASS | Saving the Qwen key clears the input; the key lives in memory only |
| PASS | Switching to DeepSeek shows its own configuration and does not change the provider before saving |
| PASS | The two keys are independent; saving the DeepSeek key does not overwrite the Qwen key |
| PASS | Neither the config nor the study backup contains either key |
| PASS | DeepSeek text and vision tests are sent to DeepSeek with its own parameters |
| PASS | DeepSeek photo recognition enters proofreading instead of inserting directly |
| PASS | DeepSeek two-page PDFs render locally and each page is recognised |
| PASS | DeepSeek produces three questions and saves a complete practice set after independent verification |
| PASS | Generation and the independent verification are both routed to the selected provider |
| PASS | DeepSeek errors do not save a half set: 401 |
| PASS | DeepSeek errors do not save a half set: 429 |
| PASS | DeepSeek errors do not save a half set: reviewfalse |
| PASS | Clearing the DeepSeek key does not affect the Qwen key |
| PASS | Without a DeepSeek key the message names the selected provider |
| PASS | Switching back to Qwen reuses the original Qwen key and model directly |
| PASS | Qwen text and vision requests keep the original protocol |
| PASS | Qwen variation practice and independent verification pass on regression |
| PASS | A refresh keeps the selected provider and both model settings, and clears the in-memory keys |
| PASS | The front end rejects a non-official DeepSeek domain and a non-default port |
| PASS | The DeepSeek settings have no horizontal overflow on a phone |
| PASS | Empty fields of the other, unsaved provider do not block saving the current provider |
| PASS | Hidden configuration is not lost; both providers' keys can be kept separately |
| PASS | DeepSeek still generates and verifies variants when the source question has an image |
| PASS | Switching providers and saving are disabled during an AI task, and background config changes are refused |
| PASS | A DeepSeek task can be cancelled and completed practice is kept |
| PASS | Direct browser calls build the address, key and parameters for the selected provider (intercepted test) |
| PASS | The new service passes a real Qwen text and vision retest |
| PASS | The service has a Qwen key but no DeepSeek key, so using the wrong credential is blocked |

The DeepSeek tests used an isolated mock service and HTTP request capture; they do **not**
represent a live DeepSeek integration. Qwen reused the key the user had authorised, and the
new service passed a real text and vision retest. The default provider remains Qwen. The two
providers never fall back to each other and neither key is exported with the study backup.

## 2026-11-11 addition: the key stays on the backend

The provider key moved from the page's memory to the local service. 22 checks passed
(64 automated tests in total, of which 14 are new).

| Result | Check |
|---|---|
| PASS | The page posts the key to `POST /api/ai-key`; the service answers `{"ok":true}` |
| PASS | The database file contains the ciphertext only — the plain key is nowhere on disk |
| PASS | The service decrypts the stored key again for each request and forwards it upstream |
| PASS | The plain key never appears in the server log or in an error message |
| PASS | A key sent in the body of `/api/chat` is ignored: the request is rejected without a call |
| PASS | `/api/status` reports `hasKey` per provider and never any key material |
| PASS | Without a stored key, `/api/chat` answers 400 with the name of the missing key |
| PASS | Saving a key clears the input field; the page keeps no key in memory |
| PASS | A reload keeps the stored key (the panel shows 「已加密保存」) |
| PASS | 「清除当前服务商密钥」 deletes the stored key and only that provider |
| PASS | The call-mode dropdown and the browser's direct provider request are gone |
| PASS | The `ai_config.transport` column is removed, including from an older database |
| PASS | A missing `SHIGUANG_SECRET` is generated on first use and written to `.env` (owner-only) |
| PASS | Another secret, a tampered ciphertext and a broken ciphertext are all rejected |
| PASS | Two owners never see each other's keys |
| PASS | Clearing the study data keeps the keys; restoring a backup does not overwrite them |
| PASS | Without the local service the page reports that the key cannot be saved |
| PASS | The static check keeps `transport`, `/chat/completions` and `aiKey` out of the page |

The key was stored with a test value only. No real provider key was used in this round, so
the live provider call was not repeated; the forwarding path itself is covered by the
automated test that asserts the decrypted key reaches the upstream call.

## 2026-11-11 addition: masked key hint and a visible current provider

8 further checks passed (69 automated tests in total).

| Result | Check |
|---|---|
| PASS | The panel shows the current provider in the heading and in a highlighted 「当前服务商」 row |
| PASS | Changing the dropdown updates that row at once and asks for a save |
| PASS | The stored key is shown in the input field as `sk-a****0XYZ`: the first and last four characters only |
| PASS | A key of twelve characters or less is masked completely (`****`) |
| PASS | The full key never appears in the page, in the source or in the backup |
| PASS | Clicking the field selects the hint, so typing replaces it |
| PASS | Saving without touching the field keeps the stored key unchanged |
| PASS | Saving a new key replaces the hint immediately — also on the 「偏好与数据」 page |
| PASS | Clearing the key removes the hint as well |
| PASS | The service builds the hint at runtime; the app file contains no key |

The hint is generated by the backend when it answers `/api/status`; it is never stored in
the study state and never reaches the database.
