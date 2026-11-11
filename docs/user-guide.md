# Shiguang Study Room · User Guide

A local workbench for turning wrong answers into mastered skills.

> The application interface is in Chinese. Chinese labels are quoted below as 「…」 so you
> can find the matching control.

## How to open it

On Windows, double-click `拾光自习室.exe`. The app opens its own window and runs a local
loopback service to save your data — there is no browser tab to manage and nothing to
install beyond the WebView2 runtime (bundled with Windows 11). Keep the window open while
you use it. See `desktop/使用说明.txt` for the full Windows instructions.

Study data is stored in the local SQLite database, not in the browser. Opening
`src/app/index.html` directly is a read-only preview: it shows the sample questions, but
changes are not saved.

Photo / PDF recognition and the "variation practice" (举一反三) feature additionally need a
network connection and a Qwen or DeepSeek API key; see [ai-guide.md](ai-guide.md). The key is
entered once in the AI settings and then kept by the local service in encrypted form, so a
reload keeps it and the browser never sees it again.

## First run

1. Work through the 18 sample questions to see how the nine subjects are organised, how
   review works, and what random practice and the statistics page look like.
2. Open 「偏好与数据」 (*Preferences & Data*) to set your name, daily goal, focus length and theme.
3. Click 「记录错题」 (*Record a wrong answer*) and fill in the question, reference answer,
   what went wrong and the knowledge point. You can attach a photo.
4. In 「今日复习」 (*Today's review*) recall each answer on your own, then rate yourself
   「还不会」 (*not yet*) / 「有点模糊」 (*a bit fuzzy*) / 「掌握了」 (*mastered*).
5. Export a full JSON backup from 「偏好与数据」 regularly. Clear the sample questions once
   you are comfortable.

## Review and points rules

| Your self-rating | Effect |
|---|---|
| 还不会 (*not yet*) | Consecutive-mastery counter resets; review again tomorrow. |
| 有点模糊 (*a bit fuzzy*) | Consecutive-mastery counter resets; review again in 2 days. |
| 掌握了 (*mastered*) | Next review after 1, 3, 7, 14 or 30 days depending on the consecutive-mastery count; stays at 30 days from the 5th time on. |

- Three consecutive 「掌握了」 ratings move a question into 「已掌握」 (*mastered*).
- A new question earns +3 points. The first review of a question on a given day earns +5.
  A completed focus session earns +1 per minute, capped at +30 per session.
- Points can be exchanged for rewards you define yourself. Redeeming does **not** reduce the
  cumulative growth value.
- Practice is self-assessed against the reference answer — there is no automatic grading.

## Saving and moving your data

- Data lives in the local service's SQLite database (`~/.local/share/shiguang-study/study.db`
  by default). It is never written back into the HTML file.
- Before moving to another machine or making major changes, export a JSON backup from
  「偏好与数据」. Import it to restore everything.
- The JSON backup contains wrong-answer images, review logs, rewards, redemption records,
  preferences and the AI provider settings. It never contains API keys: the encrypted key
  stays in the local database and the `.env` file. The CSV export is convenient
  for spreadsheets but contains no images.
- Images are compressed automatically before they are stored.
- Restoring a backup that contains a running timer leaves the timer paused, so old time is
  never counted again.
- Printing opens a separate preview page. Click 「打印 / 保存 PDF」 (*Print / Save as PDF*)
  there to open the browser's print dialog.
- Clearing the sample questions (「清空数据」) keeps the stored API keys; they are service
  credentials, not study data. Remove them in the AI settings with 「清除当前服务商密钥」.

## File independence

The app needs the local service to save data. The HTML is still a single file, but on its
own it is a read-only preview. The Go desktop app embeds the page and runs the local
loopback service itself; no Python, uv, or Node runtime is needed.

Read [ai-guide.md](ai-guide.md) before using the AI features.
