# Shiguang Study Room · 拾光自习室

[English](README.md)

一个本地优先的学习工作台：记录错题、按间隔复习、随机练习、记录专注时长，并可选让视觉模型
把照片和试卷 PDF 转成可复习的题目。它以 **单文件 Windows 桌面应用** 交付：界面是一个 HTML
文件，后端是 Go 编写的本机服务，两者编译进同一个 `.exe`。运行不需要 Python / uv / venv /
Node 运行时。

- 平台：Windows 10/11 x64
- 界面：Wails v3.1.0 + Microsoft Edge WebView2
- 存储：SQLite（`modernc.org/sqlite`，纯 Go，`CGO_ENABLED=0`）
- HTTP：只用标准库，只监听本机回环地址

应用界面为中文（面向中小学生）。标识符与代码注释使用英文；本文件是英文版
[README.md](README.md) 的中文对照版。

## 仓库结构

```
.
├── src/app/index.html      前端本体：界面 + 核心逻辑 + AI 逻辑 + 内置 PDF.js
├── desktop/                可交付的 Windows 应用（Go + Wails v3）
│   ├── web/index.html      src/app/index.html 的逐字节副本，内嵌进二进制并受哈希锁保护
│   ├── cmd/study/          入口与命令行参数
│   ├── internal/           app、config、server、store、ops、secret、llm 包
│   ├── fixtures/           Go 测试回放的黄金 JSON（冻结样本）
│   ├── build/windows/      图标、清单与注入 .syso 的版本资源
│   ├── docs/parity.md      与旧后端有意保留的行为差异
│   ├── tools/syncpage/     把 UI 真源复制进 web/ 并刷新哈希锁的同步工具
│   ├── Taskfile.yml        构建 / 测试 / 打包任务
│   └── 使用说明.txt         面向使用者的 Windows 说明
├── docs/                   用户指南、AI 指南与桌面版移植计划
└── licenses/               PDF.js（Apache-2.0）与字体/解码器许可
```

`src/app/index.html` 是界面的唯一真源。`desktop/web/index.html` 是它的逐字节副本，被 Go
二进制内嵌；`desktop/web/embed_test.go` 对其做哈希锁定。因此改动界面的流程是：先改
`src/app/index.html`，再复制到 `desktop/web/`，并**有意地**更新该测试里的
`indexSHA256`；`wails3 task sync` 会把复制与改哈希合成一步。直接用浏览器打开 HTML
（`file://`）只是只读预览：能看示例数据，但不能保存。

`desktop/internal/` 下的 Go 包职责：

| 包 | 职责 |
|---|---|
| `cmd/study` | 入口与命令行参数 |
| `internal/app` | Wails 窗口、生命周期、单实例互斥 |
| `internal/config` | 数据目录、token、owner id |
| `internal/server` | 路由、Bearer 鉴权、token 注入、错误映射 |
| `internal/store` | SQLite 连接、DDL、迁移 |
| `internal/ops` | 校验、事务、状态组装 |
| `internal/secret` | `.env` 解析、AES-256-GCM、密钥预览 |
| `internal/llm` | 端点白名单、禁重定向代理、限额 |

## 环境要求

**运行**：Windows 10/11 x64 与 WebView2 运行库（Windows 11 自带，多数 Windows 10 机器也已
安装）。

**构建**：

- Go 1.26+，且 `CGO_ENABLED=0`。本地 Go 低于 1.26 也可用：`desktop/go.mod` 声明了
  `go 1.26.0`，`GOTOOLCHAIN=auto` 会在进入 `desktop/` 目录时自动切换到所需工具链。
- Wails v3.1.0 CLI（`wails3`）需在 `PATH` 中，安装一次即可：
  `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`。二进制落在
  `$(go env GOPATH)\bin`，该目录必须在 `PATH` 中。

## 构建与测试

```bash
cd desktop
wails3 task test              # go test ./...
wails3 task vet               # go vet ./...
wails3 task sync              # 把 src/app/index.html 复制进 web/ 并刷新哈希锁
wails3 task sync:check        # 只校验副本与哈希锁是否最新
wails3 task windows:build     # build/bin/shiguang-study.exe
wails3 task package           # dist/拾光自习室/… 以及 windows-x64 zip
wails3 task serve             # 仅本机 HTTP（127.0.0.1:4312），调试用
```

`windows:build` 会先生成清单/版本 `.syso`，再用 `-H windowsgui` 构建，因此不会出现控制台
窗口；该任务以测试套件与 `sync:check` 校验为前置依赖，页面副本过期时构建会直接失败。

使用者直接双击 `拾光自习室.exe` 即可（见 [`desktop/使用说明.txt`](desktop/使用说明.txt)）。

从源码运行：

```bash
go run ./cmd/study            # 桌面窗口，随机回环端口
```

命令行参数：

| 参数 | 含义 |
|---|---|
| `--serve` | 只启动本机回环 HTTP 服务（调试） |
| `--port N` | 固定端口；`0`（默认）表示随机可用端口 |
| `--open` | 与 `--serve` 同用时，用默认浏览器打开应用 |

## 数据与兼容性

桌面版与旧 Python 后端使用相同的数据目录、表结构、token、owner id 和加密格式：

```
%USERPROFILE%\.local\share\shiguang-study\
├── study.db      # SQLite 数据库（11 张表，表结构相同）
├── token         # 只生成一次，32 字节随机数，base64url
└── .env          # 用于加密服务商密钥的 SHIGUANG_SECRET
```

旧的 Python `study.db` 可直接打开。历史遗留的 `ai_config.transport` 列会在首次打开时删除，
与旧版行为一致。服务商密钥使用相同的 AES-256-GCM 格式，因此只要 `SHIGUANG_SECRET` 相同，
旧密钥继续有效。旧版从工作目录查找 `.env`；桌面版依次读取数据目录、可执行文件目录。位于
可执行文件旁的 `.env` 视为不可信，只允许提供 `SHIGUANG_SECRET`，其余键会被忽略。若旧
`.env` 在别处（例如源码目录内），可用 `SHIGUANG_ENV_FILE` 指向它，或把它复制到数据目录。

可用环境变量覆盖：`SHIGUANG_DATA_DIR`、`SHIGUANG_DB`、`SHIGUANG_TOKEN`、
`SHIGUANG_ENV_FILE`、`SHIGUANG_SECRET`，以及 `SHIGUANG_DEV=1`（从磁盘加载
`web/index.html` 而非内嵌副本）。

## 工作原理

- **只有一个 HTML 文件。** 界面、样式、核心应用与 AI 模块都在同一个 `<script>` 块里；
  PDF.js、中文字符映射、字体与解码器以 base64/gzip 资源内嵌。页面本身没有构建步骤，
  运行时不依赖任何 CDN。
- **SQLite 是唯一数据库。** Go 服务把学习数据与非敏感的 AI 设置（服务商、模型、端点）存进
  `%USERPROFILE%\.local\share\shiguang-study\study.db`。页面不保存任何学习数据。所有写入
  都经过 `commit()`：它先校验整份状态，再把变更行发往 `/api/ops`；同步失败会提示，并在
  下次操作时重试。
- **服务商密钥留在后端。** 页面只把密钥发送一次到 `POST /api/ai-key`；服务用
  `SHIGUANG_SECRET`（AES-256-GCM）加密后，只把密文存进 SQLite。`/api/status` 只报告是否
  已配置密钥，绝不返回其值；密钥也不会进入学习备份。
- **页面从不直连服务商。** 所有模型请求都走 `/api/chat`：只接受千问与 DeepSeek 的官方
  HTTPS 端点，拒绝会携带密钥的重定向，从上游错误中脱敏密钥，并且不会在一家服务商的凭据
  失败时回退到另一家。
- **AI 产出先校验再落库。** 举一反三会发起两次独立调用——一次生成、一次校核；校核不通过
  则一组都不保存。

## 安全不变量

- 服务商 API Key 只以密文存在于 SQLite 中，绝不出现在页面、备份、日志或错误信息里。
- `/api/chat` 始终使用已存密钥，忽略请求体里的任何密钥。
- 只接受千问 / DeepSeek 官方 HTTPS 端点，不跟随重定向，只转发白名单内的请求字段。
- 服务只绑定 `127.0.0.1`。没有账号体系，也绝不能暴露到网络。
- 每个请求都必须指向回环地址；非回环 `Host`（例如 DNS 重绑定攻击）在 token 暴露之前就
  以 `403` 拒绝。
- 请求体有大小上限，服务端设置读/写/空闲超时，JSON 响应带 `Cache-Control: no-store`，
  带 token 的页面页带 `no-cache` 与 `ETag`。
- 命名互斥锁保证每个用户会话只运行一个实例。

与旧后端有意保留的行为差异见 [`desktop/docs/parity.md`](desktop/docs/parity.md)。

## 资源哈希锁

不要直接修改 `desktop/web/index.html`。任何功能改动都应落在 `src/app/index.html`，然后运行
`wails3 task sync`：它会复制文件，并**有意地**更新 `desktop/web/embed_test.go` 中的
`indexSHA256` 与 `indexSize`。否则 `go test ./web/` 与 `wails3 task sync:check` 都会失败。
`.gitattributes` 用 `* text=auto eol=lf` 把所有文本文件统一为 LF，并把两个页面文件标为
`-text`，签出时不会被改写字节，哈希锁始终有效。

## 相关文档

- [README.md](README.md) — 本文件的英文版
- [desktop/docs/parity.md](desktop/docs/parity.md) — 行为一致性与有意加固
- [docs/user-guide.md](docs/user-guide.md) — 怎么用这个工作台
- [docs/ai-guide.md](docs/ai-guide.md) — 拍照/PDF 录题、举一反三、服务商配置
- [docs/go-wails-windows-plan.md](docs/go-wails-windows-plan.md) — 桌面版移植计划（历史）

## 许可

PDF.js 是 Mozilla 项目，采用 Apache License 2.0；内置字体与解码器各自附带声明。二者均保留
在 `licenses/` 目录以及应用 HTML 内部。
