# 飞书云盘增强（Drive）

`drive` 命令组覆盖云盘文件的增强能力：大文件分块上传与原地覆盖、流式下载与断点续传、重命名、
上传文件的版本历史、异步导入导出与续查、文件夹移动、目录单向镜像与 URL/token 解析。
`doc import-file`（本地文件导入为云文档的简单入口）也归本工作流。

边界：
- 基础 CRUD（list/mkdir/copy/delete/meta/quota/版本回滚）见 `../file-media/workflow.md`；协作者、公开权限、
  申请权限（`drive apply-permission`）与密级标签（`drive secure-label`）见 `../perm/workflow.md`；评论（含 `drive add-comment`
  创建富文本/局部评论）见 `../comment/workflow.md`。
- 文档正文读取与 Markdown 导入导出（`doc export/import`、`markdown *`）属于 `feishu-cli-docs`；
  全局 `search docs/messages/apps` 与按文件夹/知识库精筛的 `drive search` 属于 `feishu-cli-platform` 的 search 工作流。

## 目录

- [身份与预检](#身份与预检)
- [命令速查](#命令速查)
- [典型工作流](#典型工作流)
- [与老命令的对照](#与老命令的对照)
- [权限要求](#权限要求)
- [注意事项](#注意事项)

## 身份与预检

| 命令 | 不传 `--as` 时的身份 | 说明 |
|---|---|---|
| `drive upload` / `download` | 必须 User Token | `--as bot\|user\|auto` 可切换 |
| `drive import/export/export-download/move/task-result/update-title/version-history/version-get` | auto | User 优先；未配置 User 时用 Bot；**已配置但解析/刷新失败 fail-closed**，不静默切 Bot |
| `drive pull/push/status` | User 优先，不可用时 stderr 告警后回退 Bot | 带 `--delete-local/--delete-remote` 时 fail-closed；`--as` 显式指定 |
| `drive inspect` | User 优先，回退 Bot | 无 `--as`；只读 |

- 续查异步任务（`task-result`、`export-download`）沿用创建任务时的身份；超时输出的 `next_command` 已带 `--as`，换身份查询常见"任务不存在/无权限"。
- User Token 缺 drive 读 scope 时返回 `99991679`（如 `drive:drive` / `drive:drive:readonly` / `space:document:retrieve` 任选其一），
  退出码 3：资源对应用可见时可显式 `--as bot`，否则按提示 `auth login --scope "..."` 补授权。
- 预检：`feishu-cli auth check --scope "drive:file:upload"`（只检查本地 User Token，不验证 Bot 与资源权限）。
- Bot 身份新建文件、导入文档后，会自动给当前 CLI 登录用户授予 `full_access`（JSON 带 `permission_grant`），规则见 `../perm/workflow.md`。

## 命令速查

### 0. 解析 URL / token

```bash
feishu-cli drive inspect --url "https://xxx.feishu.cn/docx/doxcnxxx"
feishu-cli drive inspect --url "https://xxx.feishu.cn/wiki/wikcnxxx"   # 自动 node_by_token 展开到底层文档
feishu-cli drive inspect --url doxcnxxx -o json                        # 裸 token：query_by_token 自动识别类型（含 wiki 节点）
feishu-cli drive inspect --url doxcnxxx --type docx -o json            # 裸 token + 显式类型，跳过识别
TOKEN=$(feishu-cli drive inspect --url "https://xxx.feishu.cn/wiki/wikcnxxx" -o json | jq -r '.token')
```

- `inspect` 的参数是 `--url`（也接受裸 token），不接受位置参数。输出 `type/title/token/url`；wiki 输入额外带
  `wiki_node`（node_token/obj_token/obj_type/space_id），裸 token 自动识别时带 `detected_by=query_by_token`，
  节点在回收站/已删除时带 `token_status`。

**URL 解析规则（inspect / apply-permission / export / add-comment / update-title / download，以及 import 的 `--folder-token` 通用）**：
- 只按 URL **路径前缀**识别类型：`/docx/`、`/doc/`、`/docs/`、`/sheets/`、`/spreadsheets/`、`/base/`、`/bitable/`、
  `/wiki/`、`/file/`、`/drive/file/`、`/drive/folder/`、`/drive/shr/`、`/chat/drive/`、`/mindnote(s)/`、`/slides/`；
  `?from=/wiki/xxx` 这类查询参数不会改变解析结果。
- 只接受 `*.feishu.cn` / `*.larksuite.com` / `*.larkoffice.com` 的 https 链接（私有化部署需开启 `allow_custom_base_url`）。
- 显式类型（`--type` / `--doc-type`）与 URL 推断的类型冲突时直接报错；唯一例外是 wiki URL 配合需要解包的命令
  （如 `drive export`、`drive inspect`），此时类型参数表示期望的底层文档类型。

### 1. 上传 / 下载 / 重命名 / 版本

```bash
# 上传（>20MB 自动走 upload_prepare/part/finish 分块，每片有界重试）
feishu-cli drive upload --file /tmp/report.pdf
feishu-cli drive upload --file /tmp/big.zip --folder-token fldxxx --name "年度报告.zip"
feishu-cli drive upload --file /tmp/report.pdf --folder-token fldxxx --as bot   # Bot 新建后自动给当前用户授 full_access

# 原地覆盖为新版本：file_token 不变、返回 version，协作者/公开范围等权限设置不变
feishu-cli drive upload --file /tmp/report-v2.pdf --file-token boxcnxxxx --name "report.pdf"

# 下载（流式写盘，无 100MB 上限；JSON 输出用 --output-format，因为 --output 是保存路径）
feishu-cli drive download --file-token boxcnxxxx --output ./report.pdf
feishu-cli drive download --file-token boxcnxxxx --output ./downloads/ --overwrite   # 目录：按服务端文件名保存
feishu-cli drive download --file-token "https://xxx.feishu.cn/file/boxcnxxxx"       # 接受 URL；wiki 包装的文件自动解包
feishu-cli drive download --file-token boxcnxxxx --output ./big.zip --timeout 30m    # 可选：总时长上限
feishu-cli drive download --file-token boxcnxxxx --as bot --output-format json

# 重命名（文件/文件夹/在线文档/wiki 节点；--type file 默认保留原扩展名）
feishu-cli drive update-title --url "https://xxx.feishu.cn/docx/doxcnxxx" --title "季度复盘"
feishu-cli drive update-title --token boxcnxxx --type file --title "报告-v2"          # → 报告-v2.pdf
feishu-cli drive update-title --token boxcnxxx --type file --title "x.md" --on-extension-mismatch allow
feishu-cli drive update-title --token fldcnxxx --type folder --title "归档" --dry-run

# 上传文件（type=file）的版本历史、按版本下载与回滚（version 是长数字，不是 tag）
feishu-cli drive version-history --file-token boxcnxxx -o json          # has_more 时带 next_cursor，用 --cursor 续翻
feishu-cli drive version-get --file-token boxcnxxx --version 7694404069074407133 --output ./old/
feishu-cli file version revert boxcnxxx 7694404069074407133              # 回滚（见 file-media 工作流）
```

**关键点**：
- `drive upload --file-token` 与 `--folder-token` 互斥；≤20MB 走 upload_all、>20MB 走 upload_prepare（均携带 file_token）；
  服务端未返回 `version` 时视为失败。**`--name` 缺省取本地文件名，覆盖会把远端标题一起改掉**（实测用本地
  `report2.txt` 覆盖 `report.txt` 后远端标题变为 `report2.txt`），要保留原名就显式传 `--name <原文件名>`。
- `drive download` 先调 query_by_token 识别 token：wiki 节点自动解包为底层文件；在线文档（docx/sheet/bitable/slides…）
  提示改用 `drive export`；识别失败（权限/网络）只在 stderr 告警并按原 token 继续。
- 下载文件名：`--output` 是文件路径时直接使用；省略或为**已存在的目录**时按响应头 Content-Disposition → 云盘标题 → file_token
  依次决定（`report..v2.pdf` 这类文件名合法）。目标已存在且未带 `--overwrite` 时报错退出。
- User/Bot 两种身份走同一条流式链路：遇"文件超出下载大小限制"自动切 HTTP Range 分片（8MB/片，每片有界重试并从断点续传）；
  60 秒无数据视为空闲超时，默认不设总时长，`--timeout` 只是可选上限；失败、超时或 Ctrl-C 不留半截文件、不破坏已有同名文件。
- `drive update-title`：`--type` 必须是真实类型（不符与不存在同样返回 981003）；wiki 节点用 `/wiki/` 里的节点 token +
  `--type wiki`；doc/mindnote 服务端不支持（本地直接拒绝）；`--type file` 改扩展名默认拒绝（退出码 2），确需修改加
  `--on-extension-mismatch allow`；981004 是缺编辑权限而非缺 scope；批量重命名请串行，避免 99991400 限流。
- `version-history` 每项含 `version`、`name`、`action_type`（upload/rename/delete_version/revert）等；
  `version-get` 与 `drive download` 同一条流式链路，`--output` 省略或为目录时用服务端文件名。

### 2. 文档导出（含 markdown 快捷路径）

```bash
# docx → markdown：走 docs_ai fetch，同步返回，不创建导出任务
feishu-cli drive export --token doxcnxxx --doc-type docx --file-extension markdown --output-dir ./exports

# docx → pdf：异步 export_tasks，有界轮询 10×5s，完成后自动下载；超时返回 next_command
feishu-cli drive export --token doxcnxxx --doc-type docx --file-extension pdf --output-dir ./exports

# wiki URL：先解析到底层文档再导出
feishu-cli drive export --url "https://xxx.feishu.cn/wiki/wikcnxxx" --file-extension pdf

# sheet / bitable → csv 必须指定子表
feishu-cli drive export --token shtcnxxx --doc-type sheet --file-extension csv --sub-id sheet_1
feishu-cli drive export --token bascnxxx --doc-type bitable --file-extension csv --sub-id tblxxxx
```

- 类型/格式矩阵：doc→docx/pdf；docx→docx/pdf/markdown；sheet→xlsx/csv；bitable→xlsx/csv/base；slides→pptx/pdf。
- `--url` 可替代 `--token`；裸 token 必须带 `--doc-type`；`--only-schema` 仅 bitable→base；`--output-dir` 默认当前目录。
- 完成时 JSON 带 `saved_path`、`file_name`；超时（不报错，退出码 0）时带 `ready=false`、`timed_out=true`、`ticket`、
  `next_command`（含 `--as`），按「工作流 C」续查。

### 3. `drive export-download` — 下载已完成的导出产物

```bash
feishu-cli drive export-download --file-token boxcnxxx --file-name "报告.pdf" --output-dir ./exports
feishu-cli drive export-download --file-token boxcnxxx --file-name "报告.pdf" --output-dir ./exports --overwrite
```

`--file-token` 是 `drive task-result --scenario export` 返回的**产物** file_token（不是源文档 token）。
**不传 `--file-name` 时文件名就是 file_token，没有扩展名**（实测），建议用 task-result 的 `file_name` 加上导出格式扩展名。

### 4. 文档导入

```bash
# 本地文件 → 云文档（docx / sheet / bitable / slides）
feishu-cli drive import --file report.docx --type docx
feishu-cli drive import --file data.xlsx --type sheet --folder-token fldxxx
feishu-cli drive import --file bigsheet.csv --type bitable --folder-token fldxxx
feishu-cli drive import --file deck.pptx --type slides --dry-run
feishu-cli drive import --file snapshot.base --type bitable --target-token bascnxxx   # 导入到已有多维表格
feishu-cli drive import --file report.docx --type docx --as bot -o json               # Bot 导入，JSON 带 permission_grant
```

- 官方大小矩阵（按源扩展名）：`.docx/.doc` 600MB；`.pptx` 500MB（仅 slides）；`.xlsx` 800MB（sheet/bitable）；
  `.csv` sheet 20MB / bitable 100MB；`.txt/.md/.mark/.markdown/.html/.xls/.base` 20MB（`.base` 仅 bitable）。
  >20MB 只改变上传方式（自动分片），不改变目标类型。
- 上传走临时媒体（不在云盘留中间文件）；省略 `--folder-token` 导入到根目录；`--folder-token` 解析为 wiki 节点会被拒绝。
- 有界轮询 30×2s，超时返回 `next_command`（`drive task-result --scenario import --ticket ...`）。
- `--name` 默认取本地文件名去扩展名；成功输出 `doc_token`/`url`/`type`。
- 简单入口 `feishu-cli doc import-file <local_path> --type docx|sheet|bitable [--folder fldxxx] [--name ...]` 同样走导入任务，
  写类默认 Bot（显式 `--user-access-token` 才用 User），Bot 导入同样输出 `permission_grant`；需要 slides、大文件矩阵、
  `--as` 或 resume 时用 `drive import`。

### 5. 移动（文件夹自动轮询）

```bash
# 文件移动（同步）
feishu-cli drive move --file-token doxcnxxx --type docx --folder-token fldxxx

# 文件夹移动（异步，自动轮询 task_check 30×2s）
feishu-cli drive move --file-token fldxxx --type folder --folder-token fldyyy

# 省略目标时先取真实根目录 token（GET /drive/explorer/v2/root_folder/meta）
feishu-cli drive move --file-token boxcnxxx --type file --dry-run
```

- `--type` 必填（file/docx/doc/sheet/bitable/mindnote/folder/slides）。
- 超时返回 `task_id` 与带 `--as` 的 `next_command`，用 `drive task-result --scenario task_check` 接力（任务已创建，不要重复提交）。
- 轮询期间单次查询失败视为瞬时错误继续；遇限流（99991400）立即停止并在错误里给出续查命令；每次查询都失败时报错。

### 6. 通用异步任务查询

```bash
feishu-cli drive task-result --scenario import --ticket <ticket>
feishu-cli drive task-result --scenario export --ticket <ticket> --file-token <源文档 token>
feishu-cli drive task-result --scenario task_check --task-id <task_id> --as user    # 文件夹移动 / file delete

# wiki 异步任务（wiki move-docs / move-to-drive / delete-space / delete 超时提示的续查命令）
feishu-cli drive task-result --scenario wiki_move --task-id <task_id> --as user
feishu-cli drive task-result --scenario wiki_move_to_drive --task-id <task_id> --as user
feishu-cli drive task-result --scenario wiki_delete_space --task-id <task_id> --as user
feishu-cli drive task-result --scenario wiki_delete_node --task-id <task_id> --as bot
```

- 七种 scenario：`import` / `export`（`--ticket`，export 另需源文档 `--file-token`）、`task_check` 与四个 `wiki_*`（`--task-id`）。
- 输出统一带 `ready` / `failed` / `pending`；**任务失败时命令仍以退出码 0 返回**，必须看 `failed` 字段（实测）。
  `task_check` 的失败终态同时识别 `failed` 与删除任务返回的 `fail`。
- `export` 就绪时输出产物 `file_token`、`file_name`（不含扩展名）、`file_size`；`import` 就绪时输出 `doc_token`/`doc_url`，
  以 Bot 查询时会对新文档补做一次自动授权（`permission_grant`）。
- 续查时带上原命令输出里的 `--as`。

### 7. 本地 ↔ 云盘单向镜像（pull/push/status）

把云盘文件夹与本地目录做单向镜像。**只镜像 type=file 条目**，docx/sheet/bitable/mindnote/slides/shortcut 等在线文档不参与
（没有等价本地二进制）。`--local-dir` 必须是**已存在的目录**且位于当前工作目录子树内（pull 前先 `mkdir -p`，否则报
"--local-dir 不存在或无法访问"），符号链接越界会被拒绝；落在 `~/.ssh`、`~/.feishu-cli` 等敏感目录（含 cwd 为家目录时的
`.ssh`）同样在联网前以退出码 2 拒绝，且先于 `--delete-local/--delete-remote` 的 `--yes` 确认检查。

```bash
# status：只读，四个桶 new_local / new_remote / modified / unchanged，以及 detection=exact|quick
feishu-cli drive status --folder-token fldxxx --local-dir ./mirror -o json
feishu-cli drive status --folder-token fldxxx --local-dir ./mirror --quick    # 只比修改时间，不下载远端（近似结果）

# pull：云盘 → 本地，默认 --if-exists overwrite；下载后本地 mtime 对齐远端 modified_time
mkdir -p ./mirror
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --if-exists smart   # 推荐的增量模式
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --workers 8
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --on-duplicate-remote rename
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --delete-local --yes
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --as bot     # User 缺 drive scope 时

# push：本地 → 云盘，默认 --if-exists skip；自动 create_folder 镜像目录结构
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --if-exists smart       # 远端不旧于本地时跳过
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --if-exists overwrite   # 原地覆盖全部同名文件
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --delete-remote --yes
```

**覆盖与增量**：
- `push --if-exists overwrite` 携带 file_token 原地覆盖：file_token 不变、items 带新 `version`，链接/协作者/评论/历史版本保留；
  覆盖失败直接报错，**绝不"先删后传"**；服务端未返回 version 时视为失败（改用 `--if-exists skip`）。
  它**不比较内容**，远端每个同名文件都会生成新版本（实测内容未变的文件也被覆盖）；日常增量用 `smart`。
- `--if-exists smart`：pull 在本地 mtime ≥ 远端 modified_time 时跳过；push 在远端 modified_time ≥ 本地 mtime 时跳过，否则按 overwrite 覆盖。
- push 上传前复核扫描时的本地快照（大小 + mtime），扫描后被修改的文件标记 `local_file_changed` 失败。
- `--on-duplicate-remote`：远端同一路径有多个**文件**时的处理，`fail`（默认）/ `newest` / `oldest`；pull 还支持 `rename`
  （创建最早的保留原名，其余以 `__lark_<哈希>` 后缀另存）；folder/在线文档与文件重名始终报错。

**安全语义**：
- `--delete-local` / `--delete-remote` 必须配 `--yes`，否则以退出码 10 拒绝执行；上传/下载阶段有失败时自动跳过删除阶段，
  避免「已删孤儿但部分文件没传成功」的半同步。`--delete-remote` 只删远端 type=file 孤儿，不删在线文档。
- **带 `--delete-*` 时身份 fail-closed**：已配置 User Token 但不可用（token.json 未绑定 app_id、app_id 不匹配、刷新失败）时
  直接报错而非降级 Bot——Bot 视角的远端条目更少、差集更大，`--delete-local` 会误删本地文件。修复：
  `feishu-cli auth token --bind-legacy-app --as user` 或重新 `auth login`。
- pull 按**路径段**校验远端名：`report..v2.pdf` 正常下载；整段为 `..`/`.`/空段或经符号链接指向 `--local-dir` 之外的目标会被拒绝
  （`error_class=unsafe_path`）。
- **批量失败分级**：缺 scope（99991672/99991679）、无权限（1061004/403）、限流（99991400）、参数错误（1061002/99992402）、
  父目录不存在（1061044）、服务端错误等"重跑同一批也不会成功"的错误会**终止整批**（JSON `summary.aborted=true`、
  `abort_reason`、`not_attempted`，stderr 给分类提示）；items 带 `error_class`/`code`。push 额外把冲突（1061045）、
  配额、网络错误视为终止。
- **1062507 按目录隔离**：push 命中 `1062507`（单个父文件夹直接子节点超 1500）时，把该目录标记为已满，其下（含子树）条目
  跳过并标记失败，**其余未满目录继续镜像**；收尾列出已满目录清单——先清理/归档这些文件夹或把本地文件拆到更细的子目录再重跑。

## 典型工作流

### 工作流 A：大文件分块上传

```bash
feishu-cli drive upload --file big_video.mp4 --folder-token fldxxx --name "会议录像.mp4"
# stderr 输出：分片上传: 文件大小 …, 分片大小 …, 共 N 个分片，以及逐片「分片 i/N 上传完成」
# 分片大小由服务端 upload_prepare 决定；完成后 stdout 输出 file_token
```

### 工作流 B：docx 批量导出 markdown

```bash
for doc_id in doxcn1 doxcn2 doxcn3; do
  feishu-cli drive export --token "$doc_id" --doc-type docx --file-extension markdown --output-dir ./docs
done
```

### 工作流 C：导出长文档（超时续查）

```bash
# 1. 触发导出；超时时 JSON 带 ready=false、timed_out=true、ticket、next_command（含 --as）
feishu-cli drive export --token doxcnxxx --doc-type docx --file-extension pdf --output-dir ./exports -o json > export.json
TICKET=$(jq -r '.ticket // empty' export.json)
jq -r '.next_command // empty' export.json

# 2. 续查（沿用相同 --as）；ready=true 时输出导出产物的 file_token 与 file_name
feishu-cli drive task-result --scenario export --ticket "$TICKET" --file-token doxcnxxx --as auto -o json > task.json
jq '{ready, failed, file_token, file_name}' task.json

# 3. 下载产物（显式带扩展名，否则保存为无扩展名的 file_token）
feishu-cli drive export-download --file-token "$(jq -r '.file_token' task.json)" --file-name "报告.pdf" --output-dir ./exports --as auto
```

### 工作流 D：本地 Office 文件导入为飞书文档

```bash
feishu-cli drive import --file report.docx --type docx --folder-token fldxxx
# XLSX 超过 20MB 自动分片，仍保留普通 Sheet 类型（上限 800MB）
feishu-cli drive import --file big_sheet.xlsx --type sheet --folder-token fldxxx
```

不要仅因文件超过 20MB 就改成 Bitable；目标类型以用户需求为准。CSV 上限另计：Sheet 20MB、Bitable 100MB；更大的 CSV 若需保留
Sheet，应先转换为 XLSX。

## 与老命令的对照

| 老命令 | 新 drive 命令 | 差异 |
|---|---|---|
| `file upload` | `drive upload` | file upload 写类默认 Bot；drive upload 默认 User、可 `--as`，>20MB 分块，`--file-token` 原地覆盖 |
| `file download` | `drive download` | 都是流式 + 分片重试 + 原子写；drive 额外支持 URL/wiki 解包、在线文档识别、目录默认文件名、`--overwrite`、`--as` |
| `file move` | `drive move` | drive 支持 `--as`、文件夹移动自动轮询 task_check、省略目标时取真实根目录 |
| `doc export-file`（feishu-cli-docs） | `drive export` | drive 增加 markdown 快捷路径、`--sub-id`、wiki 解析、resume |
| `doc import-file` | `drive import` | drive 支持 slides/base、官方大小矩阵、`--as`、resume |

老命令仍可用（写类默认 Bot，显式 `--user-access-token` 才切 User），需要上述增强能力时用 `drive`。

## 权限要求

| 命令 | 所需 scope |
|---|---|
| `drive upload`（含 `--file-token` 覆盖） | `drive:file:upload` |
| `drive download` | `drive:file:download`（query_by_token 识别另需 `drive:drive.metadata:readonly`，缺失只告警） |
| `drive update-title` | `drive:file:upload` 或对应类型的编辑 scope（docx/sheets/base 写权限）；`--type file` 的扩展名保护另需 `drive:drive.metadata:readonly` |
| `drive version-history` / `version-get` | `drive:file:download` |
| `drive export` | `docs:document:export`、`drive:drive.metadata:readonly`（导出 markdown 还需 `docs:document.content:read`） |
| `drive export-download` | `drive:file:download` |
| `drive import` | `docs:document:import`、`drive:file:upload` |
| `drive move` | `space:document:move` |
| `drive task-result` | `drive:drive.metadata:readonly`（`import`/`export` 另需对应导入导出 scope；`wiki_*` 需 `wiki:space:read` 或 `wiki:wiki`） |
| `drive pull` / `status` | `drive:drive.metadata:readonly`、`drive:file:download`（`status --quick` 不需要下载 scope） |
| `drive push` | `drive:drive.metadata:readonly`、`drive:file:upload`、`space:folder:create`；带 `--delete-remote` 还需 `space:document:delete` |
| `drive inspect` | `drive:drive.metadata:readonly`；wiki URL 还需 `wiki:node:read` |

各命令的身份见「[身份与预检](#身份与预检)」。

## 注意事项

- **大文件阈值**：上传与导入固定 20MB，超过自动切分片。
- **有界轮询**：导出 10×5s、导入 30×2s、文件夹移动 30×2s；超时**不报错**而是返回 `next_command`；查询瞬时失败继续轮询，
  限流立即停止并在错误中给出带 `--as` 的续查命令，全部查询失败时报错（任务已创建，续查即可，不要重复创建）。
- **退出码**：`update-title` 扩展名保护等用法错误为 2；User Token 缺 scope（99991679）为 3；`--delete-*` 未带 `--yes` 为 10；
  显式类型与 URL 冲突、目标文件已存在等本地校验失败为 1；`task-result` 查到失败任务也是 0，以 `failed` 字段为准。
- **drive move 省略 --folder-token**：先取真实根目录 token，不会把空字符串交给 move API。
