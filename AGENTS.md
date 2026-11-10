# Shiguang Study Room · AGENTS.md

## 这是什么

一个**单文件离线 Web 应用**（错题本 + 间隔复习 + 拍照/PDF 录题 + AI 举一反三），不是常规软件仓库：
无包管理器、无前端构建步骤、Python 侧只用标准库。

| 路径 | 作用 |
|---|---|
| `pyproject.toml` / `uv.lock` | **uv 项目定义**（uv_build 后端 + `shiguang-study` 命令入口）与锁文件 |
| `src/app/index.html` | **产品本体（7.5 MB）**。界面 + 核心逻辑 + AI 逻辑 + 内置 PDF.js 全在这一个文件里。改代码 = 直接改这个文件 |
| `src/shiguang_study/bridge.py` | 本地 AI 代理（纯标准库）：把 API Key 留在服务端，转发到千问 / DeepSeek |
| `scripts/build.py` | 把源码装配成 `dist/` 交付包 |
| `scripts/launch.sh` / `launch.bat` | 启动器（macOS/Linux 与 Windows） |
| `tests/test_bridge.py` | 代理单元与接口测试（无网络） |
| `tests/test_static.py` | 应用静态不变量检查 + 打包不变量检查 |
| `docs/user-guide.md` | 用户手册 |
| `docs/ai-guide.md` | AI 功能与服务商配置说明 |
| `docs/acceptance-report.md` | 历史验收记录（PASS 清单可当回归用例参考） |
| `licenses/` | PDF.js Apache-2.0 + 字体/解码器许可 |
| `dist/` | **构建产物**（可交付）：`study-room.html` + `bridge.py` + 启动器 + `licenses/` |

**界面文案、错误提示、题目示例保持中文**（面向中小学用户）；目录名、文件名、代码注释与文档为英文。
改 `src/` 后必须重新构建 `dist/`，否则交付包与源码不一致（`build.py --check` 会报错）。

## uv 管理（Python 侧）

- **只用 uv 管理依赖**：`uv add <pkg>` / `uv remove <pkg>`，**不要 pip install**，也不手写 `requirements.txt`。
- `uv.lock` 必须提交；改依赖后跑 `uv lock`。`.venv/` 不提交。
- 命令入口：`[project.scripts] shiguang-study = "shiguang_study.bridge:main"`，`uv run shiguang-study` 可直接启动。
- **交付包必须保持零依赖**：`dist/bridge.py` 是 `bridge.py` 的纯拷贝，跑在任意 Python 3.9+ 上，**不需要 uv、不需要虚拟环境**。
  因此 `bridge.py` **不得使用相对导入**（`from .`），否则拷贝进 `dist/` 后无法独立运行（`tests/test_static.py` 有断言守住）。
  端用户双击 `dist/launch.command` / `dist/launch.bat` 时也**不能**依赖 uv。

## 运行与命令

```bash
uv sync                                  # 建/更新 .venv
uv run shiguang-study                    # 起本地服务（127.0.0.1:4312，自动开浏览器）
uv run python -m shiguang_study          # 同上，模块形式
uv run python -m unittest discover -s tests
uv run python scripts/build.py           # 构建 dist/
uv run python scripts/build.py --check   # 校验 dist/ 是否与源码一致（不一致退出码 1）
uv run shiguang-study --port 5000 --no-open
python3 dist/bridge.py                   # 从交付包启动（纯标准库，无需 uv）
```
离线功能只需双击 `dist/study-room.html`（`file://`），无需 Python。

## HTML 单文件架构（改动前必读）

- `<script id="pdfjs-core" | "pdfjs-worker" | "pdfjs-resources">` 是 base64 / gzip+base64 资源块，**禁止手改**（等于重编 PDF.js）。
- 全文件只有**一个**内联 `<script>`。上半段是核心应用，下半段是 AI 模块：
  AI 段通过**重赋值包装**核心函数（`nav` / `render` / `libraryCards` / `detail` / `closeModal` / `fresh` / `validate` / `addBtn`），并用 `NAV.splice(4,0,…)` 插入「拍照录题」「举一反三」两个页面。
- **顺序敏感**：AI 段必须排在核心段之后（`addBtn` 用 `let` 声明，AI 段直接赋值）。挪到前面会崩。
- 行号只作定位参考，改动后会漂移；按内容/函数名搜索更可靠。
- 没有框架、没有组件。**所有交互走 `data-action="xxx"`**，由两个委托监听器分发（核心一个，AI 段一个）。新增按钮 = 加一个 `data-action` 分支，不要新写 `onclick`（现有 4 处 `onclick` 已被 `tests/test_static.py` 列成白名单）。
- 核心 `render()` 用 `({home:…,library:…})[view]()` 映射；AI 页面由 AI 段包装 `render` 接管，新增页面要同步改这个映射。

## 状态与持久化

- 学习数据存 localStorage key `shiguang-study-v1`（schema 1）；AI 配置存 `shiguang-ai-config-v1`（**只有服务商/模型/接口地址，没有密钥**）。
- **唯一写入口是 `commit(fn)`**：克隆 state → `validate()` → 写 localStorage → 换 `state`。校验失败抛中文错误并**保留旧数据**。
  直接改 `state.xxx` 既不持久化也不校验。
- 改数据结构必须同步改 `validate()`（AI 段另有 `validateAISets` 包装 `validate`）。备份导入同样走 `validate()`，格式不符会被拒绝。
- 图片以 data URL 存进 localStorage，受浏览器配额限制；写入失败会提示导出备份。

## 密钥红线

- API Key 只存在于内存：页面变量 `aiKey` / `aiDeepseekKey`（刷新即失效）；服务端 `DASHSCOPE_API_KEY` / `DEEPSEEK_API_KEY` 只通过 `/api/status` 返回布尔值。
- **绝不**把 Key 写进 HTML、备份 JSON、配置 localStorage、文档或任何交付物。

## 本地服务（`src/shiguang_study/bridge.py`）

- 应用页由 `find_app()` 解析，**三种布局**都要能找到：
  ① `SHIGUANG_APP` 环境变量指定的绝对路径；
  ② 与脚本同目录的 `study-room.html` / `index.html`（**交付包 dist/**）；
  ③ 向上最多 6 层里的 `src/app/index.html` 或 `dist/study-room.html`（**源码树**与**已装进 .venv 的包**）。
  改应用文件名时**必须同步检查这个解析顺序**，否则服务会返回 404 提示。
- 白名单：Host ∈ {`127.0.0.1:PORT`, `localhost:PORT`}；Origin 同集合；**POST 还需 `X-Study-Token`**（每次启动随机生成，服务端把 token 注入 HTML 的 `</head>` 前，作为 `meta[name="study-token"]`）。
- 接口白名单：千问必须 `*.aliyuncs.com` 且路径以 `/compatible-mode/v1` 结尾；DeepSeek 必须 `api.deepseek.com`（路径为空 / `/` / `/v1`）。前端 `aiValidateConfig` 与后端 `endpoint_provider` 各校验一遍。
- 只转发白名单请求字段；千问发 `enable_thinking:false`，DeepSeek 发 `thinking:{type:'disabled'}`；两家密钥**互不回退**。请求上限 25 MB、响应上限 3 MB。

## 两种调用方式（transport）

- `AI_DEFAULT.transport` 加载时自动判定：页面存在 `meta[name="study-token"]` → `proxy`（走本地服务，Key 可留在服务端环境变量）；否则 → `direct`（浏览器直连服务商，必须在页面填 Key）。
- **双击 HTML（`file://`）与本地服务（`http://127.0.0.1:4312`）是两个不同的 localStorage 域，数据不互通。**
  切换前必须先在旧位置导出 JSON 备份 → 到新位置「偏好与数据」导入。

## AI 流程中不可破坏的约束

- 举一反三 = **两次独立 API 调用**：先 `aiGenerate` 出 3 题，再 `aiReview` 独立校核；只有校核返回 `verified:true` 才落库。
  任一步失败 / 超时 / 被截断 / 题目重复 → **不保存半组练习**。不可把两步合并成一次调用。
- 识别同理：先出校对草稿，用户勾选后 `aiSaveDrafts` 才入库；已入库题不重复入库、不重复计分。
- 统一 `response_format:{type:'json_object'}` + `stream:false`；`finish_reason==='length'` 视为截断并报错。
- 取消只停止等待，**已发出的请求仍可能计费**。
- 练习为自评（还不会 / 有点模糊 / 掌握了），系统不自动判分。

## 硬性上限

照片 ≤12 MB 且仅 JPG/PNG/WebP；一批素材 ≤12 张/页；PDF ≤30 MB、≤200 页；单次选页 ≤8 页；图片压缩到长边 ≤2100 px 且 data URL ≤2.1 MB；题目/答案 ≤10000 字符；AI 练习组最多保留 30 组。

## PDF.js 运行时

core / worker 由内联 base64 在运行时解出（依赖 `Promise.withResolvers` → 需较新的 Chrome / Edge / Safari）；`pdfjs-resources` 是 gzip+base64 的 JSON，用 `DecompressionStream('gzip')` 解。改这些块 = 重编 PDF.js。

## 复习与积分规则（勿改错）

stage 0–5；「还不会」→ 次日，「有点模糊」→ 2 天后，「掌握了」→ 按 `[1,3,7,14,30]` 天（stage 5 后仍 30 天）；连续 3 次掌握 = 已掌握（stage≥3）。
新题 +3；每题当天首次复习 +5（当天再复习为 0）；专注每分钟 +1，单次上限 +30；兑换奖励**不扣**累计成长值。

## 验证

```bash
uv run python scripts/build.py --check        # dist/ 与源码一致
uv run python -m unittest discover -s tests   # 39 项测试
uv run python -m py_compile src/shiguang_study/bridge.py
```
`tests/test_static.py` 守住这些不变量：只有一个内联 `<script>`、PDF.js 资源块齐全、AI 段在核心段之后、
`onclick` 白名单、`localStorage.setItem(KEY` 只有 3 处（commit / load / 导入）、两个 storage key、
无 `sk-` 形式的密钥字面量、两家 provider 各自的 thinking 参数齐全、生成后必须经过校核、
`pyproject.toml` 声明了命令入口、`uv.lock` 存在、**`bridge.py` 不含相对导入**（保证能原样拷进 `dist/`）。
`tests/test_bridge.py` 覆盖接口白名单、请求体字段裁剪、两家 provider 参数差异、密钥脱敏、
响应过大与网络失败，以及 Host / Origin / Token / Content-Type / provider 匹配等服务端规则。

静态检查之外还需人工在浏览器验证：双击 `dist/study-room.html` 查离线功能，起本地服务查 AI 功能。
`docs/acceptance-report.md` 的 PASS 清单可作回归用例参考。改动行为后请同步更新该报告与
`docs/user-guide.md` / `docs/ai-guide.md`。

## 约定

- 应用内界面文案、代码注释、错误提示全中文；标识符用英文。
- 沿用现有的紧凑单行写法，不引入构建工具、框架、npm 依赖。
- Python 依赖只经 uv 添加；不新增 `requirements.txt`。
- 用户可见行为变更 → 同步更新 `docs/user-guide.md` / `docs/ai-guide.md`。
- 改 `src/` 后记得 `uv run python scripts/build.py` 重建 `dist/`。
