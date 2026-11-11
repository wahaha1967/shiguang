# Shiguang Study Room · AGENTS.md

**这是导航页，不是说明书。** 在这里定位方向，再去对应文档读细节。

## 这是什么

一个**单文件学习工作台**（错题本 + 间隔复习 + 拍照/PDF 录题 + AI 举一反三），以 **Windows 桌面应用**形式交付：前端单文件、无构建步骤（`src/app/index.html`）；后端用 **Go + Wails v3** 重写并编译进同一个 `.exe`，学习数据只存 SQLite。运行不需要 Python / uv / Node。

## 我要做什么 → 去哪里看

| 任务 | 先读 |
|---|---|
| 改前端（界面 / 核心逻辑 / AI 逻辑 / 内置 PDF.js） | `src/app/index.html`（唯一真源），改完同步到 `desktop/web/index.html` 并更新哈希锁（见红线） |
| 改桌面后端（HTTP / 密钥 / 数据结构 / 生命周期） | `README.md` + `desktop/internal/`（server / ops / store / secret / llm / config / app） |
| 与旧后端的行为差异、桌面版加固项 | `desktop/docs/parity.md` |
| AI 行为、服务商转发、白名单 | `desktop/internal/llm/llm.go` + `docs/ai-guide.md` |
| 复习/积分数值、硬性上限 | `docs/user-guide.md` + 前端 `index.html` |
| 跑测试、构建、打包 | `desktop/Taskfile.yml`（`wails3 task test/vet/windows:build/package/serve`） |
| 面向使用者的行为 / Windows 使用说明 | `docs/user-guide.md`、`docs/ai-guide.md`、`desktop/使用说明.txt` |

## 目录地图

| 路径 | 作用 |
|---|---|
| `README.md` / `README_zh.md` | 仓库门面：定位、结构、构建、安全与许可（英文基准 + 中文对照，两版章节一一对应） |
| `src/app/index.html` | **前端本体（约 7.5 MB）**，UI 的唯一真源：界面 + 核心逻辑 + AI 逻辑 + 内置 PDF.js |
| `desktop/web/index.html` | 上者的逐字节副本，被 Go 二进制内嵌；由 `desktop/web/embed_test.go` 哈希锁定 |
| `desktop/cmd/study/` | 入口与命令行参数（`--serve` / `--port` / `--open`） |
| `desktop/internal/server/` | 路由、Bearer 鉴权、token 注入、Host 白名单、缓存头、错误映射 |
| `desktop/internal/ops/` | 校验、事务、状态组装（落库经 `/api/ops`） |
| `desktop/internal/store/` | SQLite 连接、DDL、兼容迁移 |
| `desktop/internal/secret/` | `.env` 解析、`SHIGUANG_SECRET`、AES-256-GCM 加解密、预览 |
| `desktop/internal/llm/` | 千问 / DeepSeek 端点白名单、禁重定向转发、限额、密钥脱敏 |
| `desktop/internal/config/` | 数据目录、token、owner id |
| `desktop/internal/app/` | Wails 窗口、生命周期、单实例互斥、HTTP 服务超时 |
| `desktop/fixtures/` | 冻结的黄金 JSON 样本，被 Go 测试回放（生成脚本已随旧后端移除） |
| `desktop/build/windows/` | 图标、清单、版本资源（`wails3 generate syso` 注入） |
| `desktop/docs/parity.md` | 与旧后端的行为差异与桌面版加固项 |
| `desktop/使用说明.txt` | 面向使用者的 Windows 说明，随打包分发 |
| `desktop/Taskfile.yml` | 构建 / 测试 / 打包任务 |
| `docs/` | `user-guide` / `ai-guide` / `go-wails-windows-plan`（历史移植计划） |
| `licenses/` | PDF.js Apache-2.0 + 字体/解码器许可 |

## 运行与验证

```bash
cd desktop
wails3 task test            # go test ./...（含 fixtures 回放、哈希锁、静态断言）
wails3 task vet             # go vet ./...
wails3 task windows:build   # build/bin/shiguang-study.exe（-H windowsgui，无控制台窗口）
wails3 task package         # dist/拾光自习室/… + windows-x64 zip
wails3 task serve           # 仅本机 HTTP（127.0.0.1:4312），调试用
```

构建需 Go 1.26+（`CGO_ENABLED=0`）与 Wails v3.1.0 CLI；运行需 Windows 10/11 x64 + WebView2。
环境准备（Go 工具链自动切换、`wails3` 安装与 PATH）见根 `README.md` 的 Requirements。
双击 `src/app/index.html`（`file://`）是只读预览：能看不能存。

## 红线（违反即缺陷）

- **前端单文件 + 哈希锁**：改 UI 只改 `src/app/index.html`，再逐字节复制到 `desktop/web/index.html`，并**有意地**更新 `desktop/web/embed_test.go` 的 `indexSHA256` / `indexSize`，否则 `go test ./web/` 失败。全文件只有一个内联 `<script>`，**AI 段必须排在核心段之后**；**禁手改 PDF.js base64 资源块**。
- **唯一写入口是前端 `commit(fn)`**；改数据结构必须同步改前端 `validate()` 与后端 `desktop/internal/ops` 的校验。
- **API Key 只存后端密文**：绝不把明文 Key 写进 HTML、备份、文档、日志或错误信息；`/api/chat` 拒绝请求体注入密钥；上游错误对密钥脱敏（`[密钥已隐藏]`）。
- **服务商白名单 + 禁重定向不可放宽**：只接受千问 / DeepSeek 官方 HTTPS 端点。
- **举一反三 = 生成 + 独立校核两次调用**，任一步失败不落半组练习，不可合并。
- **复习/积分数值勿改错，硬性上限勿放宽**（见 `docs/user-guide.md`）。
- **只绑 `127.0.0.1` + Host 白名单**，无公网账号体系，不要暴露到公网。
- **桌面依赖只经 Go modules**（`desktop/go.mod`，`CGO_ENABLED=0`）；前端禁引入框架 / 构建工具 / npm。
- 界面文案、错误提示、应用内注释保持中文，标识符与仓库文档用英文；**用户可见行为变更 → 同步更新 `docs/user-guide.md` / `docs/ai-guide.md`**（Windows 使用相关同步 `desktop/使用说明.txt`）。
