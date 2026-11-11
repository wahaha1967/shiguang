# Shiguang Study Room · AGENTS.md

**这是导航页，不是说明书。** 在这里定位方向，再去对应文档读细节。
机制原理、接口参数、密钥流程、测试覆盖等全部内容在 [docs/architecture.md](docs/architecture.md)，本文只保留速查与红线。

## 这是什么

一个**单文件 Web 应用**（错题本 + 间隔复习 + 拍照/PDF 录题 + AI 举一反三）。
前端单文件、无构建步骤（`src/app/index.html`）；后端 FastAPI + SQLAlchemy + SQLite（uv 管理），学习数据只存 SQLite。

## 我要做什么 → 去哪里看

| 任务 | 先读 |
|---|---|
| 改任何代码（前端 / 后端 / 数据结构 / 密钥） | [docs/architecture.md](docs/architecture.md) §2–§7 |
| 涉及 AI 行为、服务商转发、白名单 | architecture.md §6–§7 + [docs/ai-guide.md](docs/ai-guide.md) |
| 涉及复习/积分数值、硬性上限 | architecture.md §8–§9 + [docs/user-guide.md](docs/user-guide.md) |
| 跑测试、确认某不变量有无覆盖 | architecture.md §11 + `tests/` |
| 回归对照 / 历史验收结论 | [docs/acceptance-report.md](docs/acceptance-report.md)（PASS 清单） |
| 面向使用者的行为描述 | user-guide.md / ai-guide.md |

## 目录地图

| 路径 | 作用 |
|---|---|
| `src/app/index.html` | **产品本体（7.5 MB）**。界面 + 核心逻辑 + AI 逻辑 + 内置 PDF.js，改代码 = 改这个文件 |
| `src/shiguang_study/webapp/` | FastAPI 后端（`/api/state`、`/api/ops`、`/api/import`、`/api/ai-key`、`/api/chat`、`/api/status`） |
| `src/shiguang_study/keys.py` | `.env` 读取 + `SHIGUANG_SECRET` 生成 + AES-256-GCM 加解密 |
| `src/shiguang_study/llm.py` | 千问 / DeepSeek 转发（纯标准库） |
| `tests/` | 104 项自动化测试（pytest；static / keys / llm / http / isolation / main，覆盖清单见 architecture.md §11） |
| `docs/` | architecture（开发参考）/ user-guide / ai-guide / acceptance-report |
| `pyproject.toml` / `uv.lock` | uv 项目定义与锁文件；`.env` 本地密钥文件（不入库，首次运行自动生成） |
| `licenses/` | PDF.js Apache-2.0 + 字体/解码器许可 |

## 运行与验证

```bash
uv sync                                        # 建/更新 .venv
uv run shiguang-study                          # 起服务（127.0.0.1:4312）
uv run shiguang-study --open                   # 起服务并在就绪后打开浏览器
uv run pytest                                  # 全量测试（104 项）
uv run python -m py_compile src/shiguang_study/*.py src/shiguang_study/webapp/*.py
```

双击 `src/app/index.html`（`file://`）打开是只读预览：能看不能存。

## 红线（违反即缺陷）

- **依赖只经 uv**（`uv add`/`uv remove`，开发依赖用 `uv add --dev`）；禁 pip、禁手写 `requirements.txt`、应用本身禁引入框架/构建工具/npm。
- **禁手改 PDF.js base64 资源块**；全文件只有一个内联 `<script>`，**AI 段必须排在核心段之后**（细节见 architecture.md §2）。
- **唯一写入口是 `commit(fn)`**；改数据结构必须同步改 `validate()`（§3）。
- **API Key 只存后端密文**：绝不把明文 Key 写进 HTML、备份、文档、日志或错误信息；`/api/chat` 拒绝请求体注入密钥（§5）。
- **举一反三 = 生成 + 独立校核两次调用**，任一步失败不落半组练习，不可合并（§7）。
- **复习/积分数值勿改错**，**硬性上限勿放宽**（§8–§9）。
- 默认只绑 `127.0.0.1`，无公网账号体系，**不要暴露到公网**。
- 界面文案、错误提示、注释（应用内）保持中文，标识符与仓库文档用英文；**用户可见行为变更 → 同步更新 user-guide.md / ai-guide.md**，验收相关变更同步 acceptance-report.md。
