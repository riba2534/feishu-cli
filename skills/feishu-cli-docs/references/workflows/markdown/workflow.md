# 飞书云盘原生 Markdown（markdown create/fetch/overwrite/patch/diff）

`markdown` 命令组把 Drive 上的 **`.md` 当作普通文件整体读写**，保留原始 Markdown 源码，**不做** Markdown ↔ 飞书 docx 块的转换。

## 目录

- [与 doc import/export 的区别](#与-doc-import--doc-export-的区别)
- [身份与权限](#身份与权限)
- [命令速查](#命令速查)
- [约束与踩坑](#约束与踩坑)
- [典型工作流](#典型工作流)
- [常见错误](#常见错误)
- [与 drive upload/download 的对照](#与-drive-uploaddownload-的对照)

## 与 `doc import` / `doc export` 的区别

| 命令 | 行为 | 创建出的类型 | 适用场景 |
|------|------|------------|---------|
| `doc import` | Markdown → 飞书 docx 块（标题/列表/表格/Callout/Mermaid 画板…） | docx（在线协同文档） | 给人读、要排版、要团队评论 |
| `doc export` | docx → Markdown（块解析回 markdown 源码） | 本地 `.md` | 从飞书 docx 落盘到 Git |
| `markdown create/fetch/overwrite/patch/diff` | 把 `.md` 整体上传/下载/比对，**不转换** | file（Drive 普通文件） | 原样保存 `.md` 源码、反复覆盖同一份文件、版本比对 |

**判断走哪条**：

- 想要飞书 docx 渲染（人读、排版、表格、画板）→ `doc import`（见 `../import/workflow.md`）
- 想要原始 `.md` 文本原样保留在云盘、读回完全一致 → `markdown create`
- 想反复覆盖同一份 `.md`、保持 file_token 不变（分享链接持久）→ `markdown overwrite` / `markdown patch`

## 身份与权限

- **身份**：全部子命令支持 `--as bot|user|auto`（默认 auto）。User 优先；从未配置 User Token 时回退 Bot；**已配置但解析/刷新失败 fail-closed**，不会静默切 Bot。cron/无人值守显式 `--as bot`。
- **Bot 创建自动授权**：以 Bot 身份 `create` 时，CLI 自动给当前 CLI 登录用户授予该文件 `full_access`，结果见输出的 `permission_grant`（`granted` / `skipped` / `failed`）。
- **`--dry-run`**：只打印请求计划，不解析/刷新 token，不代表已访问飞书。

| 命令 | 所需 scope（User 身份；Bot 需应用开通同名权限） |
|------|------|
| `markdown create` | `drive:file:upload`（或 `drive:drive`）；创建后查询 URL 用到元数据接口，失败只在 stderr 告警 |
| `markdown fetch` / `diff` | `drive:file:download`（或 `drive:drive`） |
| `markdown overwrite` | `drive:file:upload`；未传 `--name` 时还需读元数据（`drive:drive.metadata:readonly`） |
| `markdown patch` | `drive:file:download` + `drive:file:upload` + `drive:drive.metadata:readonly`（总会读取远端文件名） |

User 身份预检：`feishu-cli auth check --scope "drive:file:upload drive:file:download"`；缺 scope 时按提示补授。

## 命令速查

### `markdown create` — 上传新 .md

```bash
# 从本地文件创建（未传 --name 时取本地文件名）
feishu-cli markdown create --content-file ./plan.md -o json

# 从字符串创建：--name 必填；--content 不会把 "\n" 转成换行，多行内容用 $'...' 或改用 --content-file
feishu-cli markdown create --name plan.md --content $'# Plan\n\n- todo 1'

# 指定目标文件夹或 wiki 节点（二者互斥；都不传则上传到云盘根目录）
feishu-cli markdown create --content-file ./draft.md --folder-token fldxxx
feishu-cli markdown create --name draft.md --content "# wiki" --wiki-token wikcnxxx --dry-run
```

- `--content` 与 `--content-file`（别名 `--file`）二选一；`--name` 必须以 `.md` 结尾（不区分大小写）。
- `-o json` 输出 `file_token`、`file_name`、`size_bytes`、`url`（查询成功时）以及 Bot 创建时的 `permission_grant`。

### `markdown fetch` — 下载 .md

```bash
feishu-cli markdown fetch --file-token boxcnxxx                       # 打印到 stdout
feishu-cli markdown fetch --file-token boxcnxxx -o json               # JSON（含 content）
feishu-cli markdown fetch --file-token boxcnxxx --output-path ./downloads/            # 目录：用远端文件名
feishu-cli markdown fetch --file-token boxcnxxx --output-path ./local.md --overwrite  # 已存在时覆盖
feishu-cli markdown fetch --file-token boxcnxxx --version 7633658129540910621          # 历史版本
```

- 输出路径走 `--output-path`，输出格式走 `-o json`，两者可叠加。
- 不传 `--output-path` 时 `-o json` 返回 `file_token` / `file_name` / `content` / `size_bytes`；传了则返回 `saved_path` 而不含 `content`。
- `--output-path` 是已存在的目录或以 `/` 结尾时，文件名取响应头 `Content-Disposition`，缺失时回退 `<file_token>.md`。
- 本地文件已存在且未加 `--overwrite` 时报错退出，不覆盖。

### `markdown overwrite` — 覆盖已有 .md（file_token 不变）

```bash
feishu-cli markdown overwrite --file-token boxcnxxx --content-file ./new.md          # 保留远端原文件名
feishu-cli markdown overwrite --file-token boxcnxxx --content-file ./new.md --name renamed.md   # 同时改名
```

- 未传 `--name` 时一律读取远端现有文件名（含 `--content-file` 场景，**不会**拿本地文件名顶替）；读不到远端名时**非零退出且不上传**，需确认原名后显式传 `--name`。
- `-o json` 输出 `file_token`、`file_name`、`version`、`size_bytes`。
- file_token 保持不变，分享链接与权限不变；>20MB 自动分片上传，仍保留同一 file_token。

### `markdown patch` — 查找替换后覆盖

```bash
feishu-cli markdown patch --file-token boxcnxxx --pattern "TODO" --content "DONE"
feishu-cli markdown patch --file-token boxcnxxx --regex --pattern 'v([0-9]+)' --content 'v2' -o json
```

- 先下载当前内容，在本地替换**全部**命中（literal 或 RE2；`--regex` 时 `--content` 可用 `$1` 引用分组），再覆盖写回。
- `--pattern`、`--content` 必填（`--content ""` 表示删除命中文本）；替换后内容为空会被拒绝。
- `match_count=0` 时不写回，输出 `updated: false` 且退出码 0——脚本应检查 `updated` / `match_count`，不要只看退出码。
- `-o json` 输出 `updated`、`mode`、`match_count`、`version`、`size_bytes_before`、`size_bytes_after`。
- 与 overwrite 不同：patch 读不到远端文件名时会以 `<file_token>.md` 作为文件名写回，对文件名敏感时先用 `fetch -o json` 确认 `file_name`。

### `markdown diff` — 本地比对（只读，不改远端）

```bash
# 模式 1：远端（最新或 --from-version 指定版本）vs 本地文件
feishu-cli markdown diff --file-token boxcnxxx --file ./local.md

# 模式 2：远端某版本 vs 远端最新
feishu-cli markdown diff --file-token boxcnxxx --from-version 3

# 模式 3：远端版本 A vs 版本 B
feishu-cli markdown diff --file-token boxcnxxx --from-version 2 --to-version 5 --context-lines 1

# 结构化输出 + 内置 jq（无需外部 jq）
feishu-cli markdown diff --file-token boxcnxxx --file ./local.md --jq '{identical,added_lines,removed_lines}'
feishu-cli markdown diff --file-token boxcnxxx --file ./local.md --format table --jq '{identical,added_lines,removed_lines}'
```

- 缺省输出 unified diff 文本（无差异时打印 `No differences.`）；只有显式传 `--format`、`--jq` 或 `-o json` 才输出结构化结果（帮助中 `--format` 显示的默认值 json 仅在这种情况下生效）。
- 版本号必须是数字；`--to-version` 必须配合 `--from-version`，且不能与 `--file` 同用；`--file` 可与 `--from-version` 组合。
- 覆盖前先 `diff --file ./local.md` 预览改动，确认后再 overwrite，避免误覆盖。

结构化输出（顶层 9 个字段）：

```jsonc
{
  "detection": "local_vs_remote",          // local_vs_remote（模式 1）/ remote_vs_remote（模式 2/3）
  "from": "a/boxcnxxx@latest",             // 左侧：a/<token>@latest 或 a/<token>@version:N
  "to": "b/./local.md",                    // 右侧：b/<本地路径>、b/<token>@latest 或 b/<token>@version:N
  "size_bytes_before": 1024,
  "size_bytes_after": 1088,
  "identical": false,                      // 等价于 hunks 为空
  "added_lines": 5,                        // 所有 hunk 中 op="+" 的行数
  "removed_lines": 2,                      // 所有 hunk 中 op="-" 的行数
  "hunks": [
    {
      "old_start": 10, "old_lines": 4, "new_start": 10, "new_lines": 7,
      "lines": [ { "op": " ", "text": "上下文行" }, { "op": "-", "text": "旧内容" }, { "op": "+", "text": "新内容" } ]
    }
  ]
}
```

常用 `--jq`：`.identical` 判断是否一致、`.added_lines, .removed_lines` 取改动量、`.hunks[].lines[] | select(.op=="+") | .text` 拉出所有新增行。

## 约束与踩坑

1. **`.md` 后缀强制**：`create` 与 `overwrite` 的 `--name` 必须以 `.md` 结尾，否则报错。`.markdown` / `.mdx` / `.txt` 走 `drive upload`。
2. **拒绝空内容**：空字符串或空文件会被拒绝（create：`Markdown 内容为空，不支持创建空 .md 文件`；overwrite：`Markdown 内容为空，不支持把 .md 覆盖为空文件`）。需要"清空"语义时写入一个占位空格 `--content " "`。
3. **20MB 分片**：恰好 20MB 仍走单次 `upload_all`，超过 20MB 自动切 `upload_prepare/upload_part/upload_finish`；覆盖（overwrite/patch）同样携带 file_token。
4. **diff 体积上限**（在计算 diff 之前拦截，防 OOM）：每侧内容 ≤ 10MB；每侧 ≤ 20000 行，且两侧行数乘积 ≤ 2000 万。超限时报错并建议用外部 diff 工具——先用 `markdown fetch` 落盘两侧，再用本地 `diff` / `git diff`。
5. **参数错误的退出码**：`markdown` 命令的参数校验错误（缺 `--name`、后缀不对、版本号非数字等）目前以退出码 1 结束，不是 2；脚本按"非 0 即失败"处理。

## 典型工作流

### A：每天迭代同一份 `.md`

```bash
# 第一次：创建并记下 file_token
FT=$(feishu-cli markdown create --content-file ./daily-summary.md -o json | jq -r '.file_token')

# 之后：先预览差异，再覆盖（file_token 不变，分享链接持久）
feishu-cli markdown diff --file-token "$FT" --file ./daily-summary.md
feishu-cli markdown overwrite --file-token "$FT" --content-file ./daily-summary.md
```

### B：云盘 `.md` 落盘、编辑、写回

```bash
feishu-cli markdown fetch --file-token boxcnxxx --output-path ./local.md --overwrite
# 编辑 ./local.md 后写回
feishu-cli markdown overwrite --file-token boxcnxxx --content-file ./local.md
```

### C：源码备份 + 生成可读 docx

```bash
# 保留 .md 源码
feishu-cli markdown create --content-file ./design.md --folder-token fldxxx
# 再生成飞书 docx 给团队阅读（导入前按 ../import/references/doc-guide.md 检查语法）
feishu-cli doc import ./design.md --title "设计稿"
```

## 常见错误

| 触发条件 | 错误信息（节选） | 处理 |
|---|---|---|
| 当前身份对目标文件无编辑权限、缺 scope | `上传 Markdown 失败: code=<非 0>, msg=…`（含服务端 code 与 log_id） | 确认当前身份（User/Bot）对文件有编辑权限；User 身份用 `auth check --scope` 预检，Bot 确认应用已开通权限 |
| 覆盖响应未带回版本号 | `覆盖 Markdown 失败: 未返回 version` | 保留 log_id 排查；可 `fetch` 核对内容是否已更新 |
| `--name` 不以 `.md` 结尾 | `--name 必须以 .md 结尾，得到 "xxx.txt"` | 改 `.md` 后缀；其他扩展名走 `drive upload` |
| `create` 用 `--content` 但未传 `--name` | `--name 必填（使用 --content 时）` | 补 `--name xxx.md` |
| 本地文件不存在 / 是目录 | `读取本地文件失败: …` / `--content-file 必须指向文件，不是目录` | 传正确的文件路径（建议绝对路径） |
| 同时给内容与文件 / 都没给 | `--content 与 --content-file/--file 不能同时使用` / `请提供 --content 或 --content-file` | 二选一 |
| `overwrite` 未传 `--name` 且读不到远端文件名 | `无法读取 file_token=… 的现有文件名，拒绝以 ….md 静默重命名远端文件` | 确认远端原名后显式传 `--name 原名.md`，或先解决元数据读取失败（权限/scope） |
| `fetch` 目标文件已存在 | `本地文件已存在: …（使用 --overwrite 覆盖）` | 加 `--overwrite` 或换路径 |
| diff 内容过大 | `… exceeds 10.0 MB markdown +diff content limit` / `内容过大（N 行 / M 行）…` | 落盘后用外部 diff 工具 |

## 与 drive upload/download 的对照

| drive 命令 | markdown 命令 | 差异 |
|---|---|---|
| `drive upload --file x.md` | `markdown create --content-file x.md` | markdown 强制 `.md` 后缀、拒绝空内容、支持 `--wiki-token`、默认 `--as auto`；`drive upload` 不传 `--as` 时要求 User Token |
| `drive download --file-token xxx` | `markdown fetch --file-token xxx` | markdown 默认打印到 stdout，支持 `--version` 读历史版本 |
| `drive upload --file x.md --file-token <token>` | `markdown overwrite --file-token <token>` | 两者都保留 file_token、>20MB 都自动分片；`drive upload` 不传 `--name` 时用本地文件名（会把远端改名），markdown 不传 `--name` 时保留远端原名 |

`drive upload/download` 适合二进制与非 `.md` 文件（见 `feishu-cli-storage` 的 drive 工作流）；原生 Markdown 的读取、比较、查找替换与覆盖使用 `markdown` 命令组。
