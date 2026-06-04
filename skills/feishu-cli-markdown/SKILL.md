---
name: feishu-cli-markdown
description: >-
  飞书云盘原生 Markdown 文件操作（与 doc import/export 互补）。
  markdown create 上传新 .md 到云盘；markdown fetch 下载为本地 .md；
  markdown overwrite 用本地 .md 覆盖云盘已有文件（保 file_token 不变，分享链接持久）；
  markdown diff 在本地比对远端最新/历史版本，不改远端。
  当 doc 文档不适合（图床、密集代码块、版本管理）走 .md 原生文件路径。
  Drive upload_all + 自拼 Formdata multipart（SDK 不暴露 file_token field）。
  当用户请求"上传 markdown"、"下载 md"、"覆盖云盘 md"、"md diff"、"比对 markdown 版本"时使用。
argument-hint: create | fetch | overwrite | diff
user-invocable: true
allowed-tools: Bash(feishu-cli markdown:*), Bash(feishu-cli drive:*), Bash(feishu-cli auth:*), Read
---

# 飞书云盘原生 Markdown（markdown create/fetch/overwrite/diff）

`markdown` 命令组把 Drive 上的 **`.md` 当作普通文件整体读写**，保留原始 Markdown 源码，**不做** Markdown ↔ 飞书 docx 块的转换。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 核心概念：与 `doc import` / `doc export` 的区别

| 命令 | 行为 | 创建出的类型 | 适用场景 |
|------|------|------------|---------|
| `doc import` | Markdown → 飞书 docx 块（标题/列表/表格/Callout/Mermaid 画板…） | docx（在线协同文档） | 给人读、要排版、要团队评论 |
| `doc export` | docx → Markdown（块解析回 markdown 源码） | 本地 `.md` | 从飞书 docx 落盘到 Git |
| `markdown create/fetch/overwrite` | 把 `.md` 整体上传/下载，**不转换** | file（Drive 普通文件） | AI agent 直接落盘原汁原味 `.md`、保留 fenced code block 缩进、当图床/密集代码块/版本管理用 |

**判断走哪条**：

- 想要飞书 docx 渲染（人读、排版、表格分块、画板渲染）→ `doc import`
- 想要原始 `.md` 文本 unchanged 保留在云盘（AI agent 读回完全一致；docx 块解析有损）→ `markdown create`
- 想反复覆盖同一份 `.md`、保持 file_token 不变（分享链接持久）→ `markdown overwrite`

## 前置条件

- **认证**：所有 `markdown` 命令默认走 **User Access Token**（执行 `feishu-cli auth login` 登录）
- **预检**：`feishu-cli auth check --scope "drive:file:upload drive:file:download"`

## 命令速查

### 1. `markdown create` — 上传新 .md 到云盘

```bash
# 从字符串内容创建
feishu-cli markdown create --name plan.md --content "# Plan\n\n- todo 1"

# 从本地文件创建（--name 缺省时取本地 basename）
feishu-cli markdown create --content-file ./local.md
feishu-cli markdown create --file ./local.md

# 指定目标文件夹
feishu-cli markdown create --name draft.md --content-file ./tmp.md --folder-token fldxxx

# JSON 输出（拿 file_token 给后续步骤）
feishu-cli markdown create --name plan.md --content-file ./plan.md -o json
```

**关键 flag**：

| flag | 说明 |
|------|------|
| `--name` | 远端文件名，**必须 `.md` 结尾**。`create`：`--content` 时必填、`--content-file` 时可省取本地 basename。`overwrite`：`--content` 时必填、`--content-file` 时可省取本地 basename；显式传入 = 同时改名 |
| `--content` | 字符串内容（与 `--content-file` 二选一） |
| `--content-file` | 本地 `.md` 文件路径 |
| `--file` | 官方 lark-cli 兼容别名，等价于 `--content-file` |
| `--folder-token` | 目标文件夹（缺省 Drive 根目录） |
| `-o json` | JSON 输出（含 `file_token` / `file_name` / `size_bytes`） |
| `--user-access-token` | 覆盖登录态 |

**校验**：空内容直接报错（不允许创建空 `.md`）。

### 2. `markdown fetch` — 下载云盘 .md

```bash
# 打印到 stdout（缺省 --output-path 时，行为与 lark-cli markdown +fetch 一致）
feishu-cli markdown fetch --file-token boxcnxxx

# 落盘到本地路径
feishu-cli markdown fetch --file-token boxcnxxx --output-path ./local.md

# 路径已存在时强制覆盖
feishu-cli markdown fetch --file-token boxcnxxx --output-path ./local.md --overwrite

# JSON 输出（含 content 字符串）
feishu-cli markdown fetch --file-token boxcnxxx -o json
```

**关键 flag**：

| flag | 说明 |
|------|------|
| `--file-token` | Markdown 文件 token（必填） |
| `--output-path` | 本地保存路径；**目录时拼 `<fileToken>.md`**；缺省打印 stdout |
| `--overwrite` | 本地文件已存在时覆盖（缺省直接报错） |
| `-o, --output json` | JSON 输出；不传 `--output-path` 时包含 `content` 字符串 |

### 3. `markdown overwrite` — 覆盖已有 .md（保 file_token）

```bash
# 字符串覆盖
feishu-cli markdown overwrite --file-token boxcnxxx --name existing.md --content "新内容"

# 本地文件覆盖
feishu-cli markdown overwrite --file-token boxcnxxx --content-file ./new.md
feishu-cli markdown overwrite --file-token boxcnxxx --file ./new.md

# 覆盖 + 改名（必须 .md 结尾）
feishu-cli markdown overwrite --file-token boxcnxxx --content-file ./new.md --name renamed.md
```

**关键 flag**：

| flag | 说明 |
|------|------|
| `--file-token` | 目标文件 token（必填） |
| `--content` / `--content-file` | 新内容（二选一） |
| `--name` | 覆盖后文件名（`.md` 结尾；`--content` 时必填；`--content-file` 缺省使用本地 basename） |
| `-o json` | JSON 输出 |

**核心价值**：`file_token` 保持不变 → 分享链接持久、其他人收藏的链接不失效；多次迭代场景（AI agent 每天更新同一份 `.md`）优于"删了重建"。

### 4. `markdown diff` — 本地比对远端最新/历史版本（只读，不改远端）

下载远端 Markdown 内容并在本地计算 unified diff，**不修改远端文件**。三种比对模式（由参数组合决定，互斥）：

```bash
# 模式 1：远端最新 vs 本地文件
feishu-cli markdown diff --file-token boxcnxxx --file ./local.md

# 模式 2：远端某版本 vs 远端最新
feishu-cli markdown diff --file-token boxcnxxx --from-version 3

# 模式 3：远端版本 A vs 版本 B（--to-version 需配合 --from-version）
feishu-cli markdown diff --file-token boxcnxxx --from-version 2 --to-version 5 --context-lines 1

# JSON 输出结构化 hunk
feishu-cli markdown diff --file-token boxcnxxx --file ./local.md -o json
```

**关键 flag**：

| flag | 说明 |
|------|------|
| `--file-token` | 目标 Markdown 文件 token（必填） |
| `--file` | 本地 `.md` 路径（模式 1：与远端最新比对） |
| `--from-version` | 起始远端版本号（模式 2/3） |
| `--to-version` | 目标远端版本号（模式 3，需配合 `--from-version`） |
| `--context-lines` | 每个 hunk 上下保留的未变更上下文行数（默认 3） |
| `--dry-run` | 只打印比对计划，不下载/不比对 |
| `-o json` | 输出结构化 hunk；缺省打印 unified diff 文本 |

**`-o json` 输出结构**（顶层 9 字段，便于 `jq` 取改动量 / 判一致）：

```jsonc
{
  "detection": "local_vs_remote",   // 比对模式：local_vs_remote（模式 1）/ remote_vs_remote（模式 2/3）
  "from": "remote (latest)",        // 左侧来源名（模式 1="remote (latest)"；模式 2/3="remote@version=N"）
  "to": "local: ./local.md",        // 右侧来源名（模式 1="local: <path>"；模式 2="remote (latest)"；模式 3="remote@version=N"）
  "size_bytes_before": 1024,        // 左侧内容字节数
  "size_bytes_after": 1088,         // 右侧内容字节数
  "identical": false,               // 是否无差异（等价于 hunks 为空）
  "added_lines": 5,                 // 新增行数（统计所有 hunk 中 op="+"）
  "removed_lines": 2,               // 删除行数（统计所有 hunk 中 op="-"）
  "hunks": [                        // unified diff 的 hunk 数组（无差异时为空数组）
    {
      "old_start": 10, "old_lines": 4,   // 左侧起始行 / 行数
      "new_start": 10, "new_lines": 7,   // 右侧起始行 / 行数
      "lines": [
        { "op": " ", "text": "上下文行" },   // op: " "=未变 / "-"=删除 / "+"=新增
        { "op": "-", "text": "旧内容" },
        { "op": "+", "text": "新内容" }
      ]
    }
  ]
}
```

常用 `jq`：`jq '.identical'` 判是否一致、`jq '.added_lines, .removed_lines'` 取改动量、`jq '.hunks[].lines[] | select(.op=="+") | .text'` 拉所有新增行。

> 与 `overwrite` 配合：覆盖前先 `diff --file ./local.md` 预览本地相对远端最新的改动，确认后再 overwrite，避免误覆盖。

## 底层实现 & 踩坑

### 1. SDK 不暴露 `file_token` field —— 自拼 multipart

飞书 Go SDK v3.5.3 的 `UploadAllFileReqBody` 只暴露 `file_name` / `parent_type` / `parent_node` / `size` / `checksum` / `file`，**没有 `file_token`** 字段。

但官方 API `POST /open-apis/drive/v1/files/upload_all` 是支持 `file_token` 的：**带 `file_token` 时覆盖原文件保留 token、刷新 version/size；不带时按 `parent_type+parent_node` 在指定目录新建**。

为绕开 SDK 限制，`internal/client/markdown.go:OverwriteFileWithToken` 用 `client.Post` + `*larkcore.Formdata` 自己拼 multipart（translator 检测到 `*Formdata` 会自动切到 FileUpload 多部分序列化路径，见 SDK `core/reqtranslator.go:payload`）。endpoint 仍是官方 `upload_all`，参考 lark-cli `shortcuts/markdown/helpers.go:uploadMarkdownFileAll` 的写法。

### 2. 文件大小上限 ≤ 20MB

`upload_all` 单次上传 API 上限 **20MB**，本命令组只走单次上传路径：

- `markdown create` 调 `client.UploadFileWithToken`（> 20MB 时该函数会自动切到分片管线，但 `markdown create` 主场景是文本 `.md` 远小于 20MB）
- `markdown overwrite` **只支持 ≤ 20MB 小文件**，大文件覆盖需要分片接口，本命令组未实现。如果是几十 MB 的 `.md`（罕见，比如海量日志贴）走 `drive upload` 分片管线创建新文件，无法保留原 file_token。

**`markdown diff` 另有独立的行数/矩阵上限**（与 20MB 上传上限无关，防 LCS 矩阵 OOM）：

- **单侧 ≤ 20000 行**：任一侧超过即报错 `内容过大（N 行 / M 行），单侧超过行数上限 20000，建议用外部 diff 工具`
- **两侧行数乘积 ≤ 2000 万单元**：LCS 用完整 `(n+1)×(m+1)` int 矩阵，内存随两侧行数**乘积**增长；乘积超限报错 `内容过大（N × M 行），LCS 矩阵超过 20000000 单元上限（防 OOM），建议用外部 diff 工具`
- 两条都在下载完成、计算 LCS **之前**拦截（`cmd/markdown_diff.go:checkDiffSize`）。长 `.md`（海量日志贴、全量代码 dump）跑 diff 命中该报错时，改用本地 `diff` / `git diff` 等外部工具，或先用 `markdown fetch` 落盘两侧再外部比对。

### 3. `.md` 后缀强制校验

`--name`（create）和 `--name`（overwrite）都强制 **`.md` 结尾**（lowercase 检查后缀），非 `.md` 直接报错。如果想存 `.markdown` / `.mdx` 走 `drive upload` 老路径。

### 4. 空内容直接报错

`--content ""` / 空 `--content-file` 都被拒绝（"Markdown 内容为空，不支持创建/覆盖为空文件"）。需要"清空"语义的话走 `markdown overwrite --content " "` 写一个占位空格。

### 5. `fetch` 的路径输出与 JSON 输出分离

`markdown fetch` 使用 `--output-path` 保存到本地，使用 `-o json` 输出 JSON：

- `--output-path ./x.md` → 落盘到 `./x.md`
- `-o json` → 把 `{file_token, content, size_bytes}` 打到 stdout
- 两者可叠加：落盘 + 同时 stdout JSON 摘要

### 6. `--output-path` 是目录时的兜底文件名

`markdown fetch --output-path ./downloads/` 会拼成 `./downloads/<fileToken>.md`（不从响应头解析原文件名，与 `drive download` 行为一致）。要保留原文件名请自己加查询步骤再拼路径。

## 何时转走 `doc import/export`

- **要飞书 docx 渲染体验**（人读、表格分行、Mermaid 转画板、Callout 高亮）→ `doc import`（参 `feishu-cli-import` skill）
- **要从飞书 docx 落盘到 Git 仓库**（块解析回 markdown）→ `doc export`（参 `feishu-cli-export` skill）
- **要把 Markdown 转换前 sanity check**（Mermaid 花括号、表格 9×9 限制、Callout 6 种类型）→ `../feishu-cli-import/references/doc-guide.md`

## 何时转走 `drive upload`

- **大文件 > 20MB**（罕见，但比如全量代码 dump、长 log）→ `feishu-cli drive upload --file xxx.md` 走分片，会创建新 file_token
- **非 `.md` 扩展名**（`.mdx` / `.markdown` / `.txt`）→ `drive upload`

## 权限要求

| 命令 | 所需 scope |
|------|------|
| `markdown create` | `drive:file:upload`（或 `drive:drive`） |
| `markdown fetch` | `drive:file:download`（或 `drive:drive`） |
| `markdown overwrite` | `drive:file:upload` + `drive:drive.metadata:readonly`；且 User Token 对目标文件有编辑权限 |
| `markdown diff` | `drive:file:download`（或 `drive:drive`） |

**推荐做法**：执行 `feishu-cli auth login` 登录后，由 `auth check --scope "drive:file:upload drive:file:download"` 预检；缺 scope 时按提示 `auth login` 补申请。

## 典型工作流

### 工作流 A：AI agent 每天迭代同一份 `.md`

```bash
# Day 1：创建一次拿到 file_token
FT=$(feishu-cli markdown create --name daily-summary.md --content-file ./summary.md -o json | jq -r '.file_token')
echo "$FT" > ~/.cache/daily-summary.token

# Day 2+：覆盖（file_token 不变，分享链接持久）
feishu-cli markdown overwrite --file-token "$(cat ~/.cache/daily-summary.token)" --content-file ./summary.md
```

### 工作流 B：从云盘 `.md` 落盘到本地

```bash
# 读回原汁原味 Markdown 源码
feishu-cli markdown fetch --file-token boxcnxxx --output-path ./local.md --overwrite

# 编辑后写回
feishu-cli markdown overwrite --file-token boxcnxxx --content-file ./local.md
```

### 工作流 C：与 `doc import` 协作（先落盘 `.md` 再批量 import）

```bash
# 上游产出 .md 到云盘（保留源码备份）
feishu-cli markdown create --name design.md --content-file ./design.md --folder-token fldxxx

# 下游再走 doc import 生成飞书 docx 给团队阅读
feishu-cli doc import ./design.md --title "设计稿" --upload-images
```

## 注意事项

- **默认 User Access Token**：所有 `markdown` 命令未登录时统一提示 `feishu-cli auth login`
- **不做 Markdown 转换**：本命令组保留 `.md` 字节流不变，**不**做飞书 docx 块转换，**不**触发图床、画板渲染、Callout 等增强逻辑——需要这些走 `doc import`
- **`upload_all` 单次 ≤ 20MB**：`create` / `overwrite` 共用 endpoint，大文件覆盖未实现，绕路走 `drive upload`（创建新 file_token）
- **`fetch` 输出参数**：路径走 `--output-path`，格式走 `-o/--output`

> `.md` 后缀强制、空内容拒绝两条已经在「核心 flag」与「踩坑」章节出现，这里不再重复，详见 [`overwrite` flag 表](#3-markdown-overwrite--覆盖已有-md保-file_token) 和 [`踩坑 §3/§4`](#3-md-后缀强制校验)。

## 常见错误

实际报错示例（参 `cmd/markdown_overwrite.go` + `internal/client/markdown.go:OverwriteFileWithToken`）：

| 触发条件 | 错误信息 | 排查方向 |
|---|---|---|
| User Token 对目标文件无编辑权限 | `覆盖文件失败: code=<非 0>, msg=<飞书返回>`（常见 `1061004 forbidden`、`1061045 no permission`） | 在飞书云盘右键文件 → 共享 → 给当前用户加「可编辑」；或换文件所有者的 token；或检查 scope `drive:file:upload` 是否已授予 |
| 文件大小超 20MB（`--content`） | `--content 大小 <N> 字节超过 20MB API 上限` | 切分文件或换 `drive upload`（会创建新 file_token，分享链接失效） |
| 文件大小超 20MB（`--content-file`） | `--content-file 大小 <N> 字节超过 20MB API 上限，请切分或用多次 fetch+overwrite` | 同上；CLI 在读盘前 `os.Stat` 已拦截，不浪费上传带宽 |
| `--name` 不以 `.md` 结尾 | `--name 必须以 .md 结尾，得到 "xxx.txt"` | 改 `.md` 后缀；`.mdx` / `.markdown` 必须走 `drive upload` |
| `--content-file` 路径不存在 | `读取本地文件失败: open <path>: no such file or directory` | 检查路径是否相对当前目录；建议传绝对路径 |
| `--content-file` 是目录 | `--content-file 必须指向文件，不是目录` | 指向具体 `.md` 文件 |
| 既给 `--content` 又给 `--content-file` | `--content 与 --content-file 不能同时使用` | 二选一 |
| 都没给 | `请提供 --content 或 --content-file` | 至少传一个 |
| `--content` 模式漏 `--name` | `使用 --content 时必须提供 --name 指定远端文件名（保留原名请加 --name <现有文件名>.md）` | 显式 `--name existing.md`（不会自动 fallback `fileToken.md`） |
| 内容为空字节 | `Markdown 内容为空，不支持把 .md 覆盖为空文件` | 要清空语义传 `--content " "`（占位空格） |
| HTTP 层异常 | `覆盖文件失败: HTTP <status>, body: <raw>` | 网络/代理问题，附带原始 body 便于排查 |
| 响应解析失败 | `解析覆盖响应失败: <json error>` | 飞书侧返回非 JSON（极罕见，通常网关错误页） |

> 权限不足时飞书 API 直接返回 `code != 0` 的业务错（如 `1061004`、`1061045`），CLI 层透传 `msg` 字段；若同时缺 scope 会在 `auth check` 阶段就拦截，建议执行前先跑 `feishu-cli auth check --scope "drive:file:upload"`。

## 与老命令的对照

| 老命令 | 新 markdown 命令 | 差异 |
|---|---|---|
| `drive upload --file x.md` | `markdown create --content-file x.md` | markdown 强制 `.md` 后缀 + 空内容校验 + AI agent 友好 |
| `drive download --file-token xxx` | `markdown fetch --file-token xxx` | markdown 默认打印 stdout（文本场景）+ 目录路径自动拼 `.md` |
| 无 | `markdown overwrite` | 老命令没有覆盖语义（只能"删了重建"，file_token 变化） |

**老 `drive upload/download` 仍然可用**（二进制、非 `.md` 走老路径），新能力集中在 `markdown overwrite`（保 file_token 覆盖）。

## v1 PR quality-pass 加固

- **`drive/v1/files/upload_all` 单次上传 ≤ 20MB**（create / overwrite 共用 endpoint）。CLI 层在 `--content` / `--content-file` 都做 pre-check，超过直接报错不浪费带宽
- **`overwrite` 必须显式 `--name`**（用 `--content` 时）：不再 fallback `<fileToken>.md` 默默改名远端文件；保留原名请显式 `--name <existing>.md`
