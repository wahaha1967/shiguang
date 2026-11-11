# Shiguang Study Room · Photo / PDF capture and variation practice

Updated: 2026-11-11.

> The application interface is in Chinese. Chinese labels are quoted as 「…」 so you can
> find the matching control.

## 1. How to open it

On Windows, double-click `拾光自习室.exe`; it opens its own window and runs a local loopback
service that saves your study data. Keep the window open while you use the app.

1. Study data is stored in the local SQLite database, not in the page.
2. The AI features need that local service: the page never talks to the provider itself.
3. The provider key is encrypted by the service and stored in the local database.

For the AI features, configure the provider below in 「偏好与数据」 or the AI settings on the
「拍照录题」 / 「举一反三」 pages.

## 2. Choose and configure the AI provider (Qwen by default)

Open 「偏好与数据」 or the AI settings on the 「拍照录题」 / 「举一反三」 pages and pick the
**AI 服务商** (*AI provider*). Qwen is the default; DeepSeek can be selected freely.

The two providers keep separate endpoints and model settings, and separate keys — switching
never mixes them. The selected provider handles photo/PDF recognition, variant generation
and the independent review step.

**Tongyi Qwen (通义千问)**

- Default Beijing endpoint: `https://dashscope.aliyuncs.com/compatible-mode/v1`
- Singapore endpoint: `https://dashscope-intl.aliyuncs.com/compatible-mode/v1`
- A business-space specific base URL ending in `/compatible-mode/v1` is also accepted.
- Vision model for photos and PDFs: `qwen3-vl-plus`
- Text model for variant generation: `qwen-plus`. When the source question has an image, the
  vision model is used to read it before questions are generated.
- Paste an Alibaba Cloud Bailian API key from the same region as the endpoint and press
  「保存设置」 (*Save settings*). The key is sent to the local service, encrypted and stored
  in the local database.
- 「测试连接」 (*Test connection*) sends one tiny text request and one vision request, so you
  can confirm both models work.

**DeepSeek**

- Default endpoint `https://api.deepseek.com`; `https://api.deepseek.com/v1` also works.
- Vision model default `deepseek-flash` (supports image input).
- Text model default `deepseek-flash`; you may enter another text model your account has
  access to, e.g. `deepseek-v4-pro`. Keep a model that supports images in the vision field.
- Paste your own DeepSeek API key, save, then click 「测试连接」; the test checks the text
  model and the image model separately.

**Keys**

- The key lives in the local service, not in the page. The page sends it once, the
  service encrypts it with the secret from `.env` and stores the ciphertext in SQLite. The
  page never receives a key back — only the fact that one is configured.
- The panel shows a masked hint of the stored key **in the input field itself**, for example
  `sk-a****0XYZ`: the first and the last four characters only, the rest is `****`. A key of
  twelve characters or less is masked completely. The hint is produced by the service at
  runtime; no key ever appears in the HTML file, in the source or in a backup.
- Clicking the field selects the hint, so typing replaces it. Saving without touching the
  field keeps the stored key unchanged.
- The encryption secret is `SHIGUANG_SECRET` in the `.env` file in the data directory
  (`%USERPROFILE%\.local\share\shiguang-study\.env`). It is generated automatically on first
  use. **Keep that file: without it the stored keys cannot be decrypted.**
- Keys are not part of the study backup. A backup moved to another machine carries no key;
  paste the key again there.
- Get a Qwen key: <https://help.aliyun.com/zh/model-studio/get-api-key>
- Get a DeepSeek key: <https://platform.deepseek.com/api_keys>
- 「清除当前服务商密钥」 (*Clear the current provider's key*) deletes the stored key of the
  selected provider. It does not touch the other provider.
- A reload keeps the selected provider, the model settings and the stored keys. The input
  field itself is always empty after a save — the key never lingers in the page.
- A failed call never silently switches to the other provider. Check the error message and
  the permissions, key and balance of the account you selected.
- Billing follows the console of the provider you selected.

**Which provider is used**

The panel shows the current provider twice: in the heading 「AI 连接设置 · 通义千问」 and in
the highlighted row 「当前服务商」. Changing the dropdown switches that row immediately, but
the choice only takes effect after 「保存设置」 (*Save settings*) — the status line under the
buttons says so until you save.

## 3. Photos and image capture

Open 「拍照录题」 (*Capture*) and take a photo, or upload JPG / PNG / WebP files.

- On a phone the system camera opens; on desktop the app asks for camera permission and
  releases the camera immediately after the shot. If no camera is available, upload an
  existing photo instead.
- Limit: 12 MB per photo, 12 images per batch. Images are compressed locally and only
  uploaded when you click 「识别所选素材」 (*Recognise selected material*).
- After recognition, check every question: wording, answer, knowledge point and subject,
  and write down what actually went wrong for you.
- Tick the questions and click 「保存勾选题目」 (*Save selected questions*).
- With 「为缺少答案的题生成参考解答」 (*Generate a reference solution for questions that lack
  one*) enabled, the model may supply a solution; the UI marks it as AI-generated and it
  still needs your review. With it disabled, only answers visible in the picture are
  transcribed — a missing answer stays empty and must be filled in before saving.
- Watch minus signs, fractions, sub/superscripts and the conditions attached to diagrams
  closely. You can attach the original page image to the saved question.

Duplicates are reported instead of being saved twice. Questions already in the library are
never inserted or scored again. To redo a recognition, click 「重新识别」 (*Recognise again*)
next to the material — questions already saved are unaffected.

## 4. PDF capture

Upload a PDF of at most 30 MB and 200 pages, enter page numbers such as `1-3,5`, then click
「加载所选页面」 (*Load selected pages*).

- At most 8 pages per selection, and at most 12 images per batch in total.
- Both text PDFs and scanned-image PDFs are rendered to page images locally; only the ticked
  pages are sent to the vision model.
- PDF.js, the Chinese character maps, the standard fonts and the decoding resources are
  embedded in the HTML — no component is downloaded from a CDN.
- Encrypted PDFs must be decrypted first. Damaged files produce an error and never modify
  the wrong-answer library.
- If a page is too dense, or a single page holds more than 12 questions, crop or split the
  page before recognising it.

## 5. Variation practice (举一反三)

1. Click 「举一反三」 on a question card or in the question detail, or open that page and
   pick a source question.
2. Choose 「同难度」 (*same difficulty*) or 「循序渐进」 (*gradually harder*), then click
   「AI 出三道相似题」 (*Generate three similar questions*).
3. The model keeps the original question type and main solving method and produces three
   different new questions around the same knowledge point. A **second, independent model
   call** then checks that wording, answer and explanation agree. Only a set that passes is
   saved.
4. Write your own solution first, then reveal the reference answer and rate yourself
   「还不会」 / 「有点模糊」 / 「掌握了」.
5. Questions worth revisiting can be added to the wrong-answer library individually, after
   which they follow the normal review schedule.

Practice is self-assessed, not auto-graded. AI questions and answers still need checking.
Practice sets, answers, the reveal state and self-ratings are stored in the local SQLite
database and included in the full JSON backup; the 30 most recent sets are kept.

Generating and self-assessing award no points. A question added to the library earns the usual
+3 for a new question; adding it twice is blocked.

On network failure, authentication failure, rate limiting, incomplete generation, duplicate
questions or a failed review, the app reports the problem and **does not save a partial set**.
Each set costs two API calls — one to generate, one to review.

Cancelling stops the wait, but a request already sent to the provider may still be billed.

## 6. Test scope and references

The Go test suite covers the protocol, error handling, key storage and the practice flow with
simulated model responses, and replays golden fixtures captured from the original Python
release.

The key handling was verified on 2026-11-11: the page sends the key once to the local
service, the service stores only the ciphertext in SQLite (the plain key is not in the
database file), the service decrypts it again for each request, and the page can no
longer inject a key per request. A reload keeps the key.

On 2026-10-22 the real Qwen service was exercised with a user-supplied key: text and vision
connectivity, photo recognition, two-page PDF recognition, generation of three same-type
variants and the independent review all passed, and the sample answers were checked by hand.
This only means those samples passed — it is not a guarantee that recognition or question
generation is error-free for every subject or every diagram-heavy question.

The DeepSeek interface was verified with simulated responses and request-capture tests
covering switching, photo/PDF handling, generation plus independent review, error recovery
and key isolation. No real DeepSeek key was available, so no live DeepSeek call was made.

Before using your own key, click 「测试连接」 and then try one real question.

Provider documentation:

- <https://help.aliyun.com/zh/model-studio/qwen-api-via-openai-chat-completions>
- <https://help.aliyun.com/zh/model-studio/vision>
- <https://help.aliyun.com/zh/model-studio/qwen-structured-output>
- <https://api-docs.deepseek.com/>
- <https://api-docs.deepseek.com/guides/vision/>
- <https://api-docs.deepseek.com/guides/json_mode/>
- <https://api-docs.deepseek.com/guides/thinking_mode/>

PDF.js is a Mozilla project; its version and licence are preserved in the HTML and in the
`licenses/` folder of the bundle.
