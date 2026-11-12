# Shiguang Study Room · 拾光自习室

**把错题变成会题。** 一个本地优先的学生学习工作台，打包成单个 Windows `.exe`——没有云端账号，
无需安装额外运行时，也不用盯着浏览器标签页。

[English](README.md)

![Windows](https://img.shields.io/badge/Windows-10%2F11%20x64-0078D6?logo=windows&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)
![Wails](https://img.shields.io/badge/Wails-v3.1.0-DF3A01)
![SQLite](https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite&logoColor=white)

拾光自习室帮学生记录错题、按间隔复习、随机练习、记录专注时长。还可以可选地让视觉模型把照片和
试卷 PDF 转成可复习的题目，并用「举一反三」围绕同一知识点生成经过校核的相似题。

界面与后端编译进同一个可执行文件；学习数据保存在你本机的 SQLite 数据库里。双击即可开始学习。

> 应用界面为中文（面向中小学生）。本文件是中文对照版；[README.md](README.md) 是英文版。

## 功能特性

**从错误中学习**

- 覆盖九大学科的错题本——语文 · 数学 · 英语 · 物理 · 化学 · 生物 · 政治 · 历史 · 地理——
  每条记录包含题目、参考答案、错因和知识点，可附照片。
- 间隔复习：自评「还不会 / 有点模糊 / 掌握了」，下次复习分别在 1 / 3 / 7 / 14 / 30 天后
  回来。连续三次「掌握了」就把题目移入「已掌握」。
- 随机练习、统计页，以及回收站，避免误删丢失。

**保持动力**

- 专注计时器，按分钟奖励积分（每次会话有上限）。
- 成长积分——新题 +3、当天首次复习 +5、每专注分钟 +1——以及可自定义的奖励兑换。兑换积分
  不会降低累计成长值。

**快速录题（可选 AI）**

- 拍照录题：拍摄或上传 JPG/PNG/WebP，视觉模型把它转写成可复习的题目；图片在本地压缩后再上传。
- PDF 录题：文字版或扫描版试卷 PDF 用内置 PDF.js 在本地渲染，只识别你勾选的页面——不从 CDN
  下载任何组件。
- 举一反三：围绕同一知识点生成三道相似题，再由第二次独立模型调用校核题干、答案与解析，
  通过后才保存。
- 支持通义千问（默认）或 DeepSeek。两家服务商各自保留独立的密钥与设置，切换不会混用。

**数据在你自己机器上**

- 一键 JSON 备份（题目、图片、复习记录、奖励、偏好、AI 设置）与 CSV 导出；导入即可在另一台
  机器恢复。API Key 绝不进入备份。
- 打印或把任意视图保存为 PDF。
- 本地优先：无账号、无云端、只监听本机回环地址的服务。

## 快速开始

### 只想直接用？

1. 构建应用：`cd desktop && wails3 task package` 会生成
   `desktop/dist/拾光自习室/拾光自习室.exe`（以及一个 `windows-x64` zip）。前置条件见下方
   **从源码构建**。
2. 双击 `拾光自习室.exe`，学习时请保持窗口开启。若 Windows 提示「未知发布者」，选择
   「更多信息 → 仍要运行」。
3. 先做完 18 道示例题，了解各功能如何配合，然后在「偏好与数据」里设置姓名、每日目标、专注
   时长与主题。
4. 记录你自己的错题，每天复习，并时不时导出一份 JSON 备份。

只需要 Windows 10/11 x64 与 WebView2 运行库（Windows 11 自带，多数 Windows 10 机器也已安装）。
完整的使用者说明见 [`desktop/使用说明.txt`](desktop/使用说明.txt) 与
[docs/user-guide.md](docs/user-guide.md)。

### 想用 AI 功能？

拍照/PDF 识别与举一反三需要联网，以及一个千问或 DeepSeek 的 API Key：在 AI 设置里填写一次，
之后由本机服务加密保存。见 [docs/ai-guide.md](docs/ai-guide.md)。

> 直接用浏览器打开 `src/app/index.html`（`file://`）只是**只读预览**：能看示例数据，但不能
> 保存。要保存数据请运行桌面应用。

## 隐私与安全

本地优先正是重点：你的学习数据和服务商密钥都留在本机。

- 无账号、无云端——服务只绑定 `127.0.0.1`，绝不暴露到网络。
- 服务商 API Key 只以密文形式保存（AES-256-GCM），绝不出现在页面、备份、日志或错误信息里。
- 模型请求只发往千问 / DeepSeek 官方 HTTPS 端点；AI 生成的题目必须通过一次独立校核调用，
  才会被保存。

完整的请求/响应契约、校验规则与加固说明见
[`desktop/docs/backend.md`](desktop/docs/backend.md)。

## 数据存储

所有学习数据都保存在用户目录下：

```
%USERPROFILE%\.local\share\shiguang-study\
├── study.db      # 存放全部学习数据的 SQLite 数据库
├── token         # 本机服务的访问令牌
└── .env          # 用于加密服务商密钥的 secret
```

**请保留 `.env` 文件：没有它，已存的密钥无法解密。**

## 从源码构建

**运行**打包好的应用只需要 Windows 10/11 x64 + WebView2（见上）。**构建**则需要：

- Go 1.26+，且 `CGO_ENABLED=0`。本地 Go 低于 1.26 也可用：`GOTOOLCHAIN=auto` 会在进入
  `desktop/` 目录时自动切换到 `desktop/go.mod` 声明的工具链。
- Wails v3.1.0 CLI（`wails3`）需在 `PATH` 中，安装一次即可：
  `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`（落在 `$(go env GOPATH)\bin`）。

```bash
cd desktop
wails3 task package          # dist/拾光自习室/… 以及 windows-x64 zip
wails3 task windows:build    # build/bin/shiguang-study.exe
wails3 task test             # go test ./...
wails3 task serve            # 仅本机 HTTP（127.0.0.1:4312），调试用
go run ./cmd/study           # 从源码运行：桌面窗口，随机回环端口
```

开发流程——单文件前端、页面同步与哈希锁、仓库约定——见 [agents.md](agents.md)。

## 相关文档

- [README.md](README.md) — 本文件的英文版
- [docs/user-guide.md](docs/user-guide.md) — 怎么用这个工作台
- [docs/ai-guide.md](docs/ai-guide.md) — 拍照/PDF 录题、举一反三、服务商配置
- [desktop/docs/backend.md](desktop/docs/backend.md) — 后端行为与安全
- [desktop/使用说明.txt](desktop/使用说明.txt) — 面向使用者的 Windows 说明

## 许可

PDF.js 是 Mozilla 项目，采用 Apache License 2.0；内置字体与解码器各自附带声明。二者均保留在
`licenses/` 目录以及应用 HTML 内部。