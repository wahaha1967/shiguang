# Shiguang Study Room · User Guide

A local, offline-first workbench for turning wrong answers into mastered skills.

> The application interface is in Chinese. Chinese labels are quoted below as 「…」 so you
> can find the matching control.

## How to open it

Double-click `study-room.html` (or `src/app/index.html` in the source tree) and open it in
any desktop browser.

Organising, reviewing and timing your work need no login and no internet connection — the
HTML file contains the whole interface, its scripts and the PDF parsing resources.

Photo / PDF recognition and the "variation practice" (举一反三) feature do need a network
connection and a Qwen or DeepSeek API key. For those, use the bundled launcher:
`launch.command` on macOS, `launch.bat` on Windows. See [ai-guide.md](ai-guide.md).

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

- Data lives in the local storage of the browser you used. It is never written back into
  the HTML file.
- Before switching browsers, moving the file, or clearing browser data, export a JSON
  backup first. Open the new location and import it to restore everything.
- The JSON backup contains wrong-answer images, review logs, rewards, redemption records
  and preferences. The CSV export is convenient for spreadsheets but contains no images.
- Images are compressed automatically. Browser storage is limited, so when it runs out the
  app tells you and keeps the data as it was before the failed save.
- Restoring a backup that contains a running timer leaves the timer paused, so old time is
  never counted again.
- Printing opens a separate preview page. Click 「打印 / 保存 PDF」 (*Print / Save as PDF*)
  there to open the browser's print dialog.

## File independence

Offline features need nothing but the HTML file. The recommended way to call the AI models
additionally needs the local Python service and the launch scripts in the same folder (the
service uses only the Python standard library, no `pip install` required).

Read [ai-guide.md](ai-guide.md) before using the AI features, especially for the data
migration step between the double-clicked page and the local service.
