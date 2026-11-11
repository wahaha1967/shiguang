# 拾光自习室 · Go + Wails v3 Windows 桌面版实施方案

> 本文是新仓库的实施蓝图。旧仓库（Python/FastAPI）保持冻结，作为行为参考与 fixture 来源。
> 目标日期：2026-11；目标平台：Windows 10/11 x64。

---

## 0. 决策记录

| 项目 | 决策 |
|---|---|
| 交付目标 | Windows 单文件 `.exe`，zip 分发，双击即用 |
| 代码签名 | 不做（接受 SmartScreen“未知发布者”提示） |
| UI | 复用 `src/app/index.html`，**逐字节不改** |
| 后端 | Go 全量重写，保持 HTTP 契约 |
| GUI 框架 | Wails v3（不使用 v2） |
| 存储 | SQLite + `modernc.org/sqlite`（纯 Go） |
| 进程模型 | 单进程；窗口加载进程内 loopback HTTP 服务 |
| 多平台 | 不做；仅 Windows |
| 旧版数据 | 同路径同 schema，直接复用 |

---

## 1. 目标与范围

### 1.1 目标

1. 交付一个 Windows `.exe`，用户无需安装 Python、uv 或任何运行时。
2. 界面与当前浏览器版完全一致，包括交互、文案、错误提示。
3. 保留现有数据文件（`study.db`、`token`、`.env`）的兼容性。
4. 保留现有安全约束：API Key 只存后端密文、服务商白名单、仅监听本机。

### 1.2 非目标（本阶段不做）

- macOS / Linux 桌面版。
- 多用户、账号体系、公网部署。
- 前端重写（Vite/React）或 Wails Bindings 化。
- 图片懒加载、`/api/state` 瘦身。
- 自动更新、托盘、开机自启。
- 代码签名与安装包（后续可选）。

---

## 2. 技术选型（定稿）

| 组件 | 选型 | 说明 |
|---|---|---|
| 语言 | Go 1.26+ | Wails v3 的最低要求 |
| GUI | Wails v3.0.0（或经 Phase 0 验证的 3.1.x） | 固定版本，禁止 `@latest` |
| WebView | Microsoft Edge WebView2 | Win11 自带；Win10 多数已有 |
| 数据库 | `modernc.org/sqlite` | 纯 Go，`CGO_ENABLED=0`，可从 WSL 交叉编译 |
| HTTP | `net/http`（标准库） | 不引入框架 |
| 加密 | `crypto/aes` + `cipher.NewGCM` | 与 Python 版格式一致 |
| 日志 | `log/slog` | 输出到文件或 stderr |
| 配置 | 环境变量 + 数据目录 | 兼容旧变量名 |
| 测试 | `go test` + golden fixtures | 以 Python 版输出为基准 |
| 打包 | `wails3 task windows:build` | 产出单 exe |

不引入 ORM、查询构建器、Web 框架、前端构建工具。

---

## 3. 架构

```
┌───────────────── Wails 窗口（WebView2） ─────────────────┐
│  http://127.0.0.1:<随机端口>/                            │
│  web/index.html（原样复用）                              │
│        │ fetch('/api/*', Authorization: Bearer <token>)  │
└────────┼─────────────────────────────────────────────────┘
         ▼
┌───────────────── Go 单进程 ──────────────────────────────┐
│  net/http 服务（仅 127.0.0.1）                           │
│    /  /index.html      → 注入 <meta name="api-token">    │
│    /api/health         → 无鉴权健康检查                   │
│    /api/status         → 服务商密钥状态                   │
│    /api/state          → 读取整份学习状态                 │
│    /api/ops            → 批量操作（单事务）               │
│    /api/import         → 整份备份替换                     │
│    /api/ai-key         → 保存/删除加密密钥                │
│    /api/chat           → 转发到千问/DeepSeek              │
│                                                          │
│  internal/ops   → 校验、事务、状态组装                    │
│  internal/store → SQLite（11 张表，同 schema）            │
│  internal/secret→ .env 解析 + AES-256-GCM                 │
│  internal/llm   → 白名单 + 无重定向转发                   │
└──────────────────────────────────────────────────────────┘
```

### 3.1 启动顺序

1. 解析数据目录（`%USERPROFILE%\.local\share\shiguang-study`）。
2. 加载 `.env`，读取或生成 `SHIGUANG_SECRET`；读取或生成 `token`。
3. 打开 SQLite，建表并执行兼容迁移。
4. `net.Listen("tcp", "127.0.0.1:0")` 取随机端口，启动 HTTP 服务。
5. 创建 Wails 窗口，`URL = http://127.0.0.1:<port>/`。
6. 窗口关闭时：`server.Shutdown(ctx)` → 关闭数据库。

### 3.2 为什么用 loopback HTTP 而不是 Wails Bindings

- `index.html` 的 4 处 `fetch`、`apiJson()`、token 注入逻辑全部保留，前端零改动。
- 现有 6 个接口与 104 项测试语义可直接作为验收基准。
- 安全模型不变：Key 只在后端解密，浏览器永远拿不到明文。
- 代价仅是增加一个只绑 `127.0.0.1` 的端口，可接受。

---

## 4. 新仓库结构

```
shiguang-study/
├── cmd/study/main.go          # 入口：config → store → server → wails window
├── internal/
│   ├── app/                   # Wails 应用装配、窗口、生命周期、单实例
│   ├── config/                # 数据目录、环境变量、token、owner
│   ├── server/                # 路由、鉴权、token 注入、错误映射
│   ├── store/                 # 打开数据库、DDL、迁移、连接参数
│   ├── ops/                   # ReadState / ApplyOps / ReplaceState / 校验器
│   ├── secret/                # .env 解析、AES-GCM、preview
│   └── llm/                   # endpoint 白名单、转发、限额
├── web/index.html             # 从旧仓库原样复制
├── fixtures/                  # 由 Python 版生成的 golden JSON
├── build/windows/             # 图标、manifest、Wails 构建配置
├── Taskfile.yml
├── go.mod
├── go.sum
└── README.md                  # 开发与交付说明
```

Wails 初始化可用 `wails3 init` 生成骨架，然后删掉不需要的前端模板；窗口直接加载自己的 HTTP 地址，不依赖 Wails AssetServer。

---

## 5. 接口契约

所有 `/api/*`（除 `/api/health`）都要求：

```
Authorization: Bearer <token>
```

- 缺失 → `401`，正文 `{"detail":"需要访问令牌，请在页面的 AI 设置中填写"}`（FastAPI 默认格式）
- 不匹配 → `403`，正文 `{"detail":"访问令牌无效"}`
- 比较使用常量时间比较（`crypto/subtle`）

### 5.1 端点表

| 方法 | 路径 | 请求 | 成功响应 | 失败 |
|---|---|---|---|---|
| GET | `/api/health` | — | `{"app":"shiguang-study","version":3,"database":bool}` | — |
| GET | `/api/status` | — | `{"app":...,"version":3,"hasKey":bool,"providers":{qwen:{hasKey,preview},deepseek:{...}}}` | 401/403 |
| GET | `/api/state` | — | 状态对象（见 §6.2） | 401/403 |
| POST | `/api/ops` | `{"ops":[...]}` | `{"ok":true,"applied":n}` | 400 校验失败；401/403 |
| POST | `/api/import` | `{"state":{...}}` | `{"ok":true}` | 400 备份格式错误；401/403 |
| POST | `/api/ai-key` | `{"provider":"qwen|deepseek","apiKey":"..."}` | `{"ok":true,"provider":...,"hasKey":true,"preview":"..."}` | 400；401/403 |
| DELETE | `/api/ai-key/{provider}` | — | `{"ok":true,"provider":...,"hasKey":false}` | 400；401/403 |
| POST | `/api/chat` | `{"endpoint":"https://...","provider":"qwen|deepseek","request":{...}}` | 上游 JSON，状态码透传 | 400/500/502；401/403 |
| GET | `/`、`/index.html` | — | HTML，`</head>` 前注入 token meta | 404 |

### 5.2 错误格式

FastAPI 的 `HTTPException` 返回 `{"detail":"..."}`。Go 版必须保持同一 JSON 形状与中文文案，否则前端 `apiJson()` 的报错展示会变化。

### 5.3 `/api/chat` 规则

- `endpoint` 必须通过白名单（见 §8.3）；否则 `400`。
- 若请求体带 `provider`，必须与 `endpoint` 推导出的服务商一致，否则 `400`。
- 只使用数据库中已保存的密钥；**请求体中的任何 key 字段一律忽略**。
- 未保存密钥 → `400`，文案 `尚未保存所选服务商的 API Key，请在 AI 连接设置中保存`。
- 解密失败 → `500`。
- 上游状态码 400–599 原样透传；其余异常 → `502`。

---

## 6. 数据层

### 6.1 Schema

完全沿用 `db.py` 的 11 张表与字段：

`questions`、`logs`、`rewards`、`redemptions`、`prefs`、`timers`、`ai_config`、`ai_keys`、`ai_sets`、`ai_set_questions`（以及 SQLAlchemy 的元数据表，Go 版不需要）。

要求：

- 列名、类型、主键、外键、`owner_id` 索引与 Python 版一致。
- 图片仍存 `questions.image` 的 TEXT 列，上限 2,200,000 字符。
- 日期是 `YYYY-MM-DD` 字符串；`at` 是字符串；`end` 是毫秒时间戳（float）。
- `ai_set_questions.set_id` 外键 `ON DELETE CASCADE`。

### 6.2 `/api/state` 输出

字段与 `ops.read_state()` 完全一致：

```json
{
  "schema": 1,
  "questions": [{ "id","subject","title","answer","reason","tag","cause",
                  "difficulty","image","favorite","stage","due","created","demo" }],
  "trash": [],
  "logs": [{ "id","date","at","type","label","points", "(qid)","(grade)","(minutes)" }],
  "rewards": [{ "id","name","cost" }],
  "redemptions": [{ "id","name","cost","date" }],
  "prefs": { "name","goal","minutes","theme" },
  "timer": { "running","remaining","duration","(end)" },
  "aiSets": [{ "id","aiReviewed","sourceId","sourceTitle","subject","created",
               "questions":[{ "id","title","answer","explanation","knowledge",
                              "note","revealed","grade" }] }],
  "aiConfig": { "provider","base","vision","text","deepseekBase",
                "deepseekVision","deepseekText" },
  "initialized": true
}
```

细节：

- `questions` 与 `trash` 按 `ord` 排序，按 `trashed` 分流。
- `logs` 的 `qid`/`grade`/`minutes`、`timer.end`、`aiConfig` 仅在非空时出现。
- 无 `prefs` 行时返回默认 `{name:"同学",goal:5,minutes:25,theme:"light"}`。
- 无 `timer` 行时返回 `null`。
- `initialized` = 任一数据行、`prefs` 或 `aiConfig` 存在。

### 6.3 `/api/ops` 操作类型

单次上限 5000 条，整批在**一个事务**内顺序执行，任一失败则全部回滚。

| `t` | `op` | 关键字段 | 规则 |
|---|---|---|---|
| `reset` | `reset` | — | 清空该 owner 的学习数据，**保留 ai_keys** |
| `questions` | `upsert` | `id`/`row.id`、`ord`、`trashed`、`row` | 见 §7 |
| `questions` | `delete` | `id` | 仅删除属于该 owner 的行 |
| `logs` | `upsert`/`append` | `row` | 见 §7 |
| `logs` | `delete` | `id` | — |
| `rewards` | `upsert`/`delete` | `row`/`id` | `cost` 0–10000 |
| `redemptions` | `upsert`/`delete` | `row`/`id` | 需要 `date` |
| `prefs` | `replace` | `row` | 见 §7 |
| `timer` | `replace` | `row`（可为 null） | 见 §7 |
| `aiConfig` | `replace` | `row`（可为 null） | 见 §7 |
| `aiSets` | `upsert` | `row`，恰好 3 道题 | 整体替换题目 |
| `aiSets` | `delete` | `id` | 先删子表再删主表 |

### 6.4 `/api/import`

- 要求 `schema == 1`、`questions`/`logs` 为数组、`prefs` 为对象。
- 先 `clear_owner`，再按顺序 upsert 全部数据；`ai_keys` 不参与导入。
- 任一校验失败 → `400`，数据库保持原状（事务回滚）。

### 6.5 连接与并发

- DSN 或连接初始化时设置：`foreign_keys=ON`、`journal_mode=WAL`、`busy_timeout=5000`。
- 建议 `db.SetMaxOpenConns(1)`：单用户桌面应用，串行访问最简单，避免 `SQLITE_BUSY`。
- 所有写操作走显式事务。

### 6.6 兼容迁移

保留 Python 版的 `ai_config.transport` 删除逻辑：

1. `PRAGMA table_info(ai_config)`
2. 若存在 `transport`：SQLite ≥ 3.35 用 `ALTER TABLE ... DROP COLUMN`；否则按 `REBUILD_AI_CONFIG` 重建。

新装用户不会遇到；老库必须能平滑打开。

### 6.7 owner 与 token

- `owner_id = hex(sha256(token))[:16]`，与 Python 版一致。
- **不要**把 owner 改成固定值（如 `"local"`），否则旧数据会“消失”。
- token 持久化在数据目录的 `token` 文件；环境变量 `SHIGUANG_TOKEN` 优先。

---

## 7. 校验规则（必须逐条对齐）

### 7.1 基础校验器

| 校验器 | 规则 |
|---|---|
| `_ident` | `^[A-Za-z0-9_-]{1,100}$` |
| `_date` | `^\d{4}-\d{2}-\d{2}$` |
| `_text` | 必须是字符串；长度上限按**字符数**（Unicode 码点）计算，不是字节数 |
| `_bool` | 必须是布尔值 |
| `_int` | 必须是 JSON 整数；范围检查；拒绝 `true/false` 和带小数点的数字 |
| `_number` | 必须是 JSON 数字；范围检查；拒绝布尔值 |

> **Go 实现注意**：`encoding/json` 默认把所有数字解成 `float64`，会丢失“整数/小数”的区别。必须使用 `Decoder.UseNumber()`，并用 `json.Number` 的原始字面量判断是否为整数，才能与 Python 的 `isinstance(value, int)` 行为一致。文本长度用 `utf8.RuneCountInString`。

### 7.2 各表字段

**questions.upsert**

| 字段 | 规则 |
|---|---|
| `subject,title,answer,reason,tag,cause,difficulty` | text，≤10000 |
| `image` | text，≤2,200,000 |
| `favorite,demo` | bool，默认 false |
| `stage` | int 0–5 |
| `due,created` | date，默认 `1970-01-01` |
| `trashed` | bool，默认 false |
| `ord` | int 0–1,000,000 |
| id 冲突 | 若 id 已属于其他 owner → `记录编号已被使用` |

**logs.upsert**

| 字段 | 规则 |
|---|---|
| `date` | date |
| `at` | text ≤40 |
| `type` | `add/review/practice/focus` |
| `label` | text ≤10000 |
| `points` | number 0–30 |
| `qid,grade` | 可空；`review/practice` 必须有 `grade ∈ again/hard/good` |
| `minutes` | 可空；number 0–1440 |
| `ord` | int 0–1,000,000 |

**rewards.upsert / redemptions.upsert**

| 字段 | 规则 |
|---|---|
| `name` | text ≤10000 |
| `cost` | int 0–10000 |
| `ord` | int 0–1,000,000 |
| `date`（仅 redemptions） | date |

**prefs.replace**

| 字段 | 规则 |
|---|---|
| `name` | text ≤20 |
| `goal` | int 1–100 |
| `minutes` | int 1–120 |
| `theme` | `light/night` |

**timer.replace**

| 字段 | 规则 |
|---|---|
| `running` | bool |
| `duration` | number 0–86400 |
| `remaining` | number 0–86400，且 ≤ duration |
| `end` | 可空；number 0–4,102,444,800,000 |
| `row = null` | 删除 timer 行 |

**aiConfig.replace**

| 字段 | 规则 |
|---|---|
| `provider` | `qwen/deepseek` |
| `base,deepseekBase` | text ≤500 |
| `vision,text,deepseekVision,deepseekText` | text ≤120 |
| `row = null` | 删除配置行 |

**aiSets.upsert**

| 字段 | 规则 |
|---|---|
| `questions` | 必须恰好 3 道 |
| `sourceId` | text ≤100 |
| `sourceTitle,subject` | text ≤10000 |
| `created` | date |
| `aiReviewed` | bool |
| 子题 | `title/answer/explanation` ≤10000；`knowledge` ≤200；`note` ≤5000；`grade` ≤20；`revealed` bool |

### 7.3 明确差异（需在实现时决定并记录）

Python 版对 `logs.qid` 与 `logs.grade` 不做类型校验，直接写库。Go 版建议：

- 允许 `null` 或字符串，拒绝其他类型；
- 该差异属于防御性收窄，不影响前端合法请求；
- 在 `docs/parity.md` 中记录。

---

## 8. 密钥与 AI 转发

### 8.1 `.env` 与 secret

- 查找顺序（桌面版）：
  1. `SHIGUANG_ENV_FILE`
  2. 数据目录 `.env`
  3. 可执行文件所在目录 `.env`（只读，不写入）
- 解析规则与 Python `_parse_env` 一致：忽略空行/`#`、支持 `export `、支持成对引号。
- secret 缺失时：生成 32 字节随机值，`base64.RawURLEncoding`，写入数据目录 `.env`（并追加中文注释），**永不重新生成已有值**。
- Windows 上 `chmod 0600` 语义有限；文件位于用户目录即接受。文档中说明。

### 8.2 加密格式（必须逐字节兼容）

```
key        = sha256(secret_utf8)
nonce      = 12 random bytes
ciphertext = AES-256-GCM(key, nonce, plaintext_utf8, nil)
stored     = base64(nonce || ciphertext)
```

- 必须能解开 Python 版生成的密文。
- `preview`：长度 ≤12 → `****`；否则前 4 + `****` + 后 4（按字符计）。

### 8.3 服务商白名单

只接受：

- `https`
- 无 userinfo、无 query、无 fragment
- 端口为空或 443
- 千问：host 为 `aliyuncs.com` 或其子域，path 以 `/compatible-mode/v1` 结尾
- DeepSeek：host 为 `api.deepseek.com`，path 为 `""` 或 `/v1`
- 其他一律 `400`，文案 `仅接受服务商官方 HTTPS 接口基础地址`

### 8.4 转发

- URL = base 去掉尾部 `/` + `/chat/completions`
- 仅允许字段：`model`、`messages`、`response_format`、`max_tokens`、`stream`
- 强制 `stream=false`
- 千问：`enable_thinking=false`；DeepSeek：`thinking={"type":"disabled"}`
- 请求上限 25 MB，响应上限 3 MB，超限 → `502`
- 超时 100 秒
- **禁止重定向**：`http.Client.CheckRedirect = func(...) error { return http.ErrUseLastResponse }`
- 上游错误信息中若含 key，替换为 `[密钥已隐藏]`，截断 500 字符
- 连接失败/超时 → `502`，文案 `无法连接所选服务商，请检查网络和接口设置后重试`

---

## 9. 桌面壳（Wails）细节

### 9.1 窗口

- 标题：`拾光自习室`
- 初始尺寸：建议 1280×860，最小 960×640
- 禁止控制台窗口：`-H windowsgui`
- 应用图标与版本信息在 `build/windows/` 配置

### 9.2 加载方式

```go
// 示意：具体字段以固定的 Wails v3 版本 API 为准
app := application.New(application.Options{Name: "拾光自习室"})
app.Window.NewWithOptions(application.WebviewWindowOptions{
    Title:  "拾光自习室",
    Width:  1280,
    Height: 860,
    URL:    fmt.Sprintf("http://127.0.0.1:%d/", port),
})
app.Run()
```

Phase 0 必须确认：Wails v3 在加载外部 `http://127.0.0.1` 时不强制要求 AssetServer / 前端模板。

### 9.3 单实例

- 使用 Windows 命名互斥量（`golang.org/x/sys/windows`）或数据目录锁文件。
- 第二个实例启动时提示“拾光自习室已在运行”，然后退出。
- 避免两个进程同时写同一个 SQLite 文件造成用户困惑。

### 9.4 生命周期

- 窗口关闭 → `server.Shutdown` → `db.Close()`。
- 数据库关闭前执行 `PRAGMA wal_checkpoint(TRUNCATE)`（可选）。

### 9.5 开发/发布资源

- 开发：`SHIGUANG_DEV=1`，从磁盘读取 `web/index.html`，改完刷新窗口生效。
- 发布：`//go:embed web/index.html`，单文件内嵌。
- 两种模式都必须注入 token meta，逻辑一致。

---

## 10. 测试策略

### 10.1 Golden fixtures（核心手段）

在旧仓库（Python）中生成，提交到新仓库 `fixtures/`：

1. **ops → state**：覆盖 7 类 op、`reset`、`import`、边界值、非法输入、双 owner 隔离。
   格式：
   ```json
   {
     "name": "questions-upsert-basic",
     "initial": { ...state... },
     "ops": [ ... ],
     "expected": { ...state... }
   }
   ```
2. **keys**：`{ "secret": "...", "ciphertext": "...", "plaintext": "..." }`，Go 测试解密后断言一致。
3. **llm**：合法/非法 endpoint、上游错误脱敏、响应超限的 fake upstream 用例。

Go 测试直接读取 fixture，执行后与 `expected` 做 JSON 深比较。这样 Windows 上不需要 Python。

### 10.2 单元测试

- `internal/ops`：每个校验器、每个 upsert、事务回滚。
- `internal/secret`：`.env` 解析、加密/解密、preview 边界（12 字符）。
- `internal/llm`：白名单、重定向拒绝、限额、脱敏。
- `internal/server`：`httptest` + 临时数据库，覆盖 6 个接口与 401/403/400。

### 10.3 静态资源测试

移植 `test_static.py`：

- 全文件只有一个内联 `<script>`
- AI 段必须排在核心段之后
- 前端只使用规定的 6 个接口
- `web/index.html` 的 SHA-256 与记录值一致（防止 UI 被误改）

### 10.4 手工验收（必须）

见 §13。

---

## 11. 打包与交付

### 11.1 构建

```powershell
$env:CGO_ENABLED = "0"
wails3 task windows:build
```

要求：

- 纯 Go SQLite，确保不引入 cgo。
- WebView2 策略：`embed`（内嵌 bootstrapper，约 +150 KB）。
- 产物：`build/bin/shiguang-study.exe`。

### 11.2 交付物

```
拾光自习室/
├── 拾光自习室.exe
└── 使用说明.txt
```

`使用说明.txt` 内容：

- 双击开始使用；首次启动可能提示“未知发布者”，选择“更多信息 → 仍要运行”。
- 数据保存在 `%USERPROFILE%\.local\share\shiguang-study`。
- 备份/恢复通过应用内“偏好与数据”完成。
- AI Key 只保存在本机，请勿分享数据目录。

### 11.3 不做的交付项

- 不做代码签名。
- 不做安装包（后续可选 Inno Setup）。
- 不做自动更新。

---

## 12. 阶段计划

### Phase 0 · 桌面壳验证（1–2 天）

交付：

- `wails3 init` 骨架、`cmd/study/main.go`、随机端口、token 注入、`/api/health` 与假 `/api/state`、`/api/status`。
- 窗口加载 `index.html`，UI 可渲染（空数据/演示数据）。

验收：

- [ ] Windows 上双击 exe 能开窗，无控制台
- [ ] UI 与浏览器版一致
- [ ] 摄像头预览可用
- [ ] 图片/PDF 文件选择可用
- [ ] 备份导出（blob 下载）可用
- [ ] WSL 交叉编译 Windows exe 可行（若在 WSL 构建）

> 任何一项失败，先调整方案再进入 Phase 1。

### Phase 1 · 数据层与 ops（4–6 天）

交付：

- `store`：建表、迁移、owner 隔离、PRAGMA。
- `ops`：`ReadState`、`ApplyOps`、`ReplaceState`、全部校验器。
- `server`：`/api/state`、`/api/ops`、`/api/import`。
- fixtures 生成脚本与 Go 测试。

验收：

- [ ] 全部 ops fixture 通过
- [ ] 非法输入返回与 Python 相同的中文错误
- [ ] 事务回滚用例通过
- [ ] 老 `study.db` 直接打开并读出相同 state

### Phase 2 · 密钥与 AI（2–3 天）

交付：

- `secret`：`.env`、AES-GCM、preview。
- `llm`：白名单、转发、限额、脱敏。
- `server`：`/api/ai-key`、`/api/chat`、`/api/status`。

验收：

- [ ] Go 能解开 Python 生成的密文
- [ ] 千问/DeepSeek 各跑通一次文本与视觉请求
- [ ] 非法 endpoint、请求体注入 key、重定向均被拒绝

### Phase 3 · 测试移植与打包（3–5 天）

交付：

- 104 项测试的 Go 对应版本（可合并为表驱动）。
- 静态资源测试与 HTML 哈希锁。
- Wails 构建配置、图标、版本信息、zip 产物。

验收：

- [ ] `go test ./...` 全绿
- [ ] `wails3 task windows:build` 产物可运行
- [ ] 干净虚拟机验收通过（见 §13）

### Phase 4 · 交付验收与文档（1–2 天）

交付：

- 干净 Win10/11 VM 验收记录。
- `README.md`、用户说明、变更记录。
- 旧仓库打 tag 归档。

验收：

- [ ] 全量验收清单通过
- [ ] 文档与实际行为一致

**总计：约 2–3 周**（单人、聚焦）。

---

## 13. 验收清单（Definition of Done）

### 13.1 功能

- [ ] 新增/编辑/删除错题，刷新后数据仍在
- [ ] 回收站、收藏、筛选、搜索正常
- [ ] 复习流程与积分与浏览器版一致
- [ ] 拍照录入（摄像头）正常
- [ ] PDF/图片导入、OCR、举一反三（生成 + 独立校核）正常
- [ ] AI Key 保存后显示掩码预览；删除后失效
- [ ] 备份导出/导入正常
- [ ] 计时器跨重启保持

### 13.2 交付环境

- [ ] 干净 Win11，无 Go/Python
- [ ] 干净 Win10 22H2，无 WebView2（验证 bootstrapper）
- [ ] 中文用户名路径（`C:\Users\张三\...`）
- [ ] 从网络下载 zip 后运行（Mark-of-the-Web / SmartScreen）
- [ ] 数据目录与 Python 版一致，老数据可读
- [ ] 卸载/删除 exe 后用户数据仍在

### 13.3 安全

- [ ] API Key 不出现在 HTML、备份、日志、错误信息中
- [ ] `/api/chat` 忽略请求体注入的密钥
- [ ] 非官方 endpoint 被拒绝
- [ ] 重定向被拒绝
- [ ] 服务仅监听 `127.0.0.1`
- [ ] 两个实例不会同时运行

---

## 14. 从旧仓库迁移的步骤

1. 旧仓库打 tag：`python-final`，不再修改。
2. 在 WSL 的旧仓库中生成 fixtures：
   - ops/state 对照数据
   - 密钥密文样本
   - LLM fake upstream 用例
3. 新仓库初始化：
   ```powershell
   wails3 init -n shiguang-study
   go mod edit -go=1.26
   go get modernc.org/sqlite
   ```
4. 复制 `src/app/index.html` → `web/index.html`，计算 SHA-256 写入静态测试。
5. 复制 `docs/architecture.md`、`user-guide.md`、`ai-guide.md` 作为行为参考，按桌面版更新：
   - 删除 `file://` 只读预览说明
   - 删除 `http://127.0.0.1:4312` 启动说明
   - 增加桌面版启动、数据目录、SmartScreen 说明
6. 按 Phase 0 → 4 实施。

---

## 15. 风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| Wails v3 加载外部 URL 受限 | Phase 0 阻塞 | Phase 0 首日验证；备选：Wails AssetServer + 反向代理到内部 handler |
| WebView2 摄像头权限 | 拍照不可用 | v3 权限配置 + Windows 实测；失败则退回文件选择 |
| WebView2 下载（备份导出） | 备份不可用 | Phase 0 实测；失败则加一个 Go binding 保存文件（前端小改） |
| `modernc.org/sqlite` 性能 | 大批量导入慢 | 单用户数据量小；批量操作放单事务；必要时换 `zombiezen.com/go/sqlite` |
| 字符长度/整数解析与 Python 不一致 | 校验行为偏差 | `UseNumber()` + 按 rune 计数；golden fixture 覆盖边界 |
| 图片内联导致 `/api/state` 过大 | 首屏慢 | 本次不修；记录为已知问题，后续做懒加载 |
| 中文用户名/路径 | 启动失败 | 验收必测；路径全程使用 Go 标准库，不拼接 shell 命令 |
| 未签名 exe 被 SmartScreen 拦 | 交付摩擦 | 使用说明写明绕过步骤；后续可买证书 |
| 单实例缺失 | 数据竞争/困惑 | 命名互斥量；Phase 3 必须完成 |
| WSL 交叉编译失败 | 开发效率 | 切 Windows 原生开发；Phase 0 验证两种方式 |

---

## 16. 待确认事项

1. 应用正式名称与图标（当前为“拾光自习室”）。
2. 是否保留一个 `--serve` 调试模式（只起 HTTP 不开窗），便于排查。建议保留，默认关闭。
3. 旧版用户规模：若有外部用户，Phase 2 后先发一个 beta 验证数据兼容；若只有你自己，可直接全量替换。
4. 是否在本阶段同步做图片懒加载。建议不做，迁移期间保持行为一致。

---

## 17. 关键不变量（实现时不可破坏）

1. `web/index.html` 不做功能性修改；任何必要修改必须单独评审并更新哈希。
2. API Key 只以密文存在于 SQLite；浏览器、备份、日志、错误信息中不得出现明文。
3. `/api/chat` 只使用后端存储的密钥，忽略请求体中的密钥字段。
4. 服务商 endpoint 白名单与“禁止重定向”不可放宽。
5. 复习/积分数值、硬性上限不变（这些在前端，后端只做存储与校验）。
6. 服务只绑定 `127.0.0.1`，不监听 `0.0.0.0`。
7. 所有用户可见文案保持中文；标识符与仓库文档用英文。
8. 交付物中不包含 Python 运行时、`uv`、虚拟环境或构建脚本。
