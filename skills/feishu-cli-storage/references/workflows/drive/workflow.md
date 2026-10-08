# 飞书云盘增强（Drive）

`drive` 命令组提供与老 `file` / `media` / `comment add` 命令**并存**的增强能力：
分块上传、markdown 快捷导出、异步任务 resume、富文本评论、wiki 链接解析。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 目录

- [前置条件](#前置条件)
- [命令速查](#命令速查)
- [典型工作流](#典型工作流)
- [与老命令的对照](#与老命令的对照)
- [权限要求](#权限要求)
- [注意事项](#注意事项)

## 前置条件

- **认证**：多数 drive 命令必需 User Token（先 `feishu-cli auth login`）。`drive import/export/export-download/move/update-title/version-history/version-get/task-result` 支持 `--as bot|user|auto`（默认 auto：User 优先；未配置回退 Bot；已配置但刷新失败 fail-closed）。`drive download/upload/pull/push/status` 与 `file list` 也支持 `--as`，**不传时保持各自旧默认**（download/upload 必须 User；pull/push/status/file list 为 User 优先、不可用时告警回退 Bot；`--delete-local/--delete-remote` 下 fail-closed）。User Token 缺 `drive:drive`/`space:document:retrieve` 等 scope 时（99991679），可显式 `--as bot` 走应用身份。各命令的 Token 策略以「[权限要求](#权限要求)」表为唯一权威。
- **预检**：`feishu-cli auth check --scope "drive:file:upload"` 可验证 scope

## 命令速查

### 0. 元数据 / 权限申请（v1.29+ 新增 ⭐）

```bash
# 解析任意文档 URL → 输出 type/title/canonical token（自动展开 wiki）
feishu-cli drive inspect --url "https://xxx.feishu.cn/docx/doxcnxxx"
feishu-cli drive inspect --url "https://xxx.feishu.cn/wiki/wikcnxxx"   # 自动 wiki node_by_token
feishu-cli drive inspect --url doxcnxxx -o json                        # 裸 token：query_by_token 自动识别类型
feishu-cli drive inspect --url doxcnxxx --type docx -o json            # 裸 token + 显式类型

# 向文档所有者申请权限（埋藏 API，飞书文档站未收录但服务端可用）
# 必需 User Token + docs:permission.member:apply scope（或 drive:drive 等任一大权限）
feishu-cli drive apply-permission --token "<url 或裸 token>" --perm view --remark "申请理由"
feishu-cli drive apply-permission --token doxcnxxx --type docx --perm edit --remark "..."
feishu-cli drive apply-permission --token <url> --perm view --dry-run    # 预览请求
```

详见 [`embedded-api-discovery.md`](../../../../feishu-cli-platform/references/workflows/api/references/embedded-api-discovery.md)（埋藏 API 调研方法论）。

**URL 解析规则（inspect / apply-permission / export / add-comment / import 的 `--folder-token` 通用）**：
- 只按 URL **路径前缀**识别类型：`/docx/`、`/doc/`、`/docs/`、`/sheets/`、`/spreadsheets/`、`/base/`、`/bitable/`、
  `/wiki/`、`/file/`、`/drive/file/`、`/drive/folder/`、`/drive/shr/`、`/chat/drive/`、`/mindnote(s)/`、`/slides/`；
  `?from=/wiki/xxx` 这类查询参数不会改变解析结果。
- 只接受 `*.feishu.cn` / `*.larksuite.com` / `*.larkoffice.com` 的 https 链接（私有化部署需开启 `allow_custom_base_url`）。
- `--type` / `--doc-type` 与 URL 推断的类型冲突时直接报错（旧版本会静默以 `--type` 覆盖）；
  唯一例外是 wiki URL 配合需要解包的命令（如 `drive export`、`drive inspect`），此时 `--type` 表示期望的底层文档类型。

### 1. 上传 / 下载 / 重命名 / 版本

```bash
# 上传（>20MB 自动走 3 段式分块 upload_prepare/upload_part/upload_finish）
feishu-cli drive upload --file /tmp/report.pdf
feishu-cli drive upload --file /tmp/big.zip --folder-token fldxxx --name "年度报告.zip"
feishu-cli drive upload --file /tmp/report.pdf --folder-token fldxxx --as bot   # Bot 上传，自动给当前用户授 full_access

# 原地覆盖：把已有文件更新为新版本（file_token 不变、返回新 version，权限设置保持不变；>20MB 走分片覆盖）
feishu-cli drive upload --file /tmp/report.pdf --file-token boxcnxxxx

# 下载（流式写盘，无 100MB 上限；分片失败有界重试并断点续传；60s 空闲超时；临时文件 + rename）
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

# 上传文件的版本历史与按版本下载（version 不是 tag）
feishu-cli drive version-history --file-token boxcnxxx -o json          # has_more 时带 next_cursor，用 --cursor 续翻
feishu-cli drive version-get --file-token boxcnxxx --version 7694404069074407133 --output ./old/
feishu-cli file version revert boxcnxxx 7694404069074407133              # 回滚到该版本
```

**关键点**：
- `drive upload` 分块上传每片独立重试 3 次，使用 `io.SectionReader` 外层只打开文件一次
- `drive upload --file-token`：把本地文件覆盖为该文件的新版本，file_token 不变；**不会改变文件已有权限设置**（协作者、公开范围等保持原样）。≤20MB 走 upload_all、>20MB 走 upload_prepare（均携带 file_token）；服务端未返回 version 时视为失败。与 `--folder-token` 互斥
- `drive download` 下载前先调 query_by_token 识别 token：wiki 节点自动解包为底层文件；在线文档（docx/sheet/bitable/slides…）直接提示改用 `drive export`；识别失败（权限/网络）只在 stderr 告警并按原 token 继续
- `drive download` 的 `--output` 可以是文件路径（直接用）或已存在的目录；省略或为目录时文件名按 **响应头 Content-Disposition → 云盘标题 → file_token** 依次决定（文件名含 `..` 子串如 `report..v2.pdf` 合法）
- User/Bot 两种身份都走同一条流式链路：遇"文件超出下载大小限制"自动切 HTTP Range 分片（8MB/片，每片最多重试 3 次并从断点续传）；默认不设总时长，只要持续有数据就不会超时，`--timeout` 仅作为显式总时长上限
- 下载失败、超时或 Ctrl-C 时不会留下半截文件，也不会删除/破坏本地已有的同名文件
- `drive update-title`：`--type` 必须是真实类型（不符与不存在同样返回 981003）；wiki 节点用 `/wiki/` 里的节点 token + `--type wiki`；doc/mindnote 服务端不支持（本地直接拒绝）；981004 是缺编辑权限而非缺 scope

### 2. 文档导出（含 markdown 快捷路径）

```bash
# docx → markdown：走 POST /open-apis/docs_ai/v1/documents/{token}/fetch（format=markdown）
feishu-cli drive export --token docxxxx --doc-type docx --file-extension markdown --output-dir ./exports

# docx → pdf：走异步 export_tasks，有界轮询 10×5s，超时返回 next_command
feishu-cli drive export --token docxxxx --doc-type docx --file-extension pdf --output-dir ./exports

# sheet → csv 指定 sheet_id
feishu-cli drive export --token sheetxxxx --doc-type sheet --file-extension csv --sub-id sheet_1

# bitable → csv 指定 table_id
feishu-cli drive export --token basexxxx --doc-type bitable --file-extension csv --sub-id tblxxxx
```

**支持的格式**：
- `--doc-type`: `doc` / `docx` / `sheet` / `bitable` / `slides` / `wiki`（wiki 先 node_by_token）
- `--file-extension`: `docx` / `pdf` / `xlsx` / `csv` / `markdown` / `base` / `pptx`
- 矩阵：doc→docx/pdf；docx→docx/pdf/markdown；sheet→xlsx/csv；bitable→xlsx/csv/base；slides→pptx/pdf
- `--url` 可替代 `--token`；`--only-schema` 仅 bitable→base

**超时后的 resume 流程**：
```bash
# drive export 超时会输出：
# next_command: feishu-cli drive task-result --scenario export --ticket abc --file-token xxx

# 1. 轮询任务状态
feishu-cli drive task-result --scenario export --ticket abc --file-token xxx

# 2. 任务完成后下载产物
feishu-cli drive export-download --file-token boxxxx --output-dir ./exports
```

### 3. `drive export-download` — 下载已完成的导出文件

```bash
feishu-cli drive export-download --file-token boxxxx --output-dir ./exports
feishu-cli drive export-download --file-token boxxxx --file-name "报告.pdf" --overwrite
```

### 4. 文档导入

```bash
# 本地文件 → 云文档（docx / sheet / bitable / slides）
feishu-cli drive import --file report.docx --type docx
feishu-cli drive import --file data.xlsx --type sheet --folder-token fldxxx
feishu-cli drive import --file bigsheet.csv --type bitable --folder-token fldxxx
feishu-cli drive import --file deck.pptx --type slides
# --as bot（或 auto 未登录）导入成功后自动给当前 CLI 登录用户授予新文档 full_access，JSON 带 permission_grant
feishu-cli drive import --file snapshot.base --type bitable --target-token bascnxxx
```

**关键技术点**：
- 走 **官方 `/medias/upload_all` 端点**（`parent_type=ccm_import_open` + `extra`），**省略 parent_node**；>20MB 走 `upload_prepare/part/finish` 且 **显式 `parent_node=""`**
- `import_tasks` **始终携带** `point.mount_type=1`；省略 `--folder-token` 时 `mount_key` 为空（根目录）
- wiki 节点不能当 `--folder-token`（会先 probe `wiki node_by_token`）
- 官方大小矩阵：`.docx/.doc` 600MB、`.pptx` 500MB、`.xlsx` 800MB、`.csv` sheet 20MB / bitable 100MB、`.txt/.md/.html/.xls/.base` 20MB
- 有界轮询 30×2s，超时返回 `next_command`

### 5. 移动（文件夹自动轮询）

```bash
# 文件移动（同步，立即返回）
feishu-cli drive move --file-token boxxxx --type docx --folder-token fldxxx

# 文件夹移动（异步，自动轮询 task_check 30×2s）
feishu-cli drive move --file-token fldxxx --type folder --folder-token fldyyy

# 省略目标时先取真实 root token（GET /drive/explorer/v2/root_folder/meta）
feishu-cli drive move --file-token boxxxx --type file
```

**关键点**：
- **文件夹移动自动轮询**，不再是"发出去就不管了"
- 超时会返回 `task_id` 与带 `--as` 的 `next_command`，可用 `drive task-result --scenario task_check` 接力
- 轮询期间单次查询失败视为瞬时错误继续轮询；遇限流（99991400）立即停止并在错误里给出续查命令；每次查询都失败时报错（任务已创建，续查即可，不要重复创建）

### 6. 富文本评论（最强命令）

```bash
# 全局评论
feishu-cli drive add-comment --doc doccnxxxx --content '[{"type":"text","text":"需要修改标题"}]'

# 通过 docx URL
feishu-cli drive add-comment --doc "https://xxx.feishu.cn/docx/doccnxxxx" \
  --content '[{"type":"text","text":"评论内容"}]'

# 通过 wiki URL（自动解析到真实 docx）
feishu-cli drive add-comment --doc "https://xxx.feishu.cn/wiki/nodxxxx" \
  --content '[{"type":"text","text":"收到"}]'

# 局部评论（锚定到 docx block）
feishu-cli drive add-comment --doc doccnxxxx --block-id blk_xxx \
  --content '[{"type":"text","text":"这段重写"}]'

# 富文本：文本 + 提及用户 + 链接混合
feishu-cli drive add-comment --doc doccnxxxx --content '[
  {"type":"text","text":"请 "},
  {"type":"mention_user","mention_user":"ou_xxx"},
  {"type":"text","text":" 查看 "},
  {"type":"link","link":"https://feishu.cn"}
]'

# 电子表格单元格评论（--block-id <sheetId>!<cell>，必填）
feishu-cli drive add-comment --doc shtcnxxxx --type sheet --block-id a281f9!D6 \
  --content '[{"type":"text","text":"这个数需要核对"}]'

# 多维表格记录评论（--block-id <table-id>!<record-id>!<view-id>，必填）
feishu-cli drive add-comment --doc "https://xxx.feishu.cn/base/bascnxxxx" \
  --block-id tblxxx!recxxx!vewxxx --content '[{"type":"text","text":"请补充"}]'

# 幻灯片元素评论（--block-id <slide-block-type>!<xml-id>，必填）
feishu-cli drive add-comment --doc "https://xxx.feishu.cn/slides/sldxxxx" \
  --block-id shape!bPq --content '[{"type":"text","text":"配色再调一下"}]'

# 云盘文件全文评论（服务端仅支持部分扩展名，如 .md/.txt/.json/.csv/.pptx/.png/.jpg/.zip）
feishu-cli drive add-comment --doc boxcnxxxx --type file --content '[{"type":"text","text":"已阅"}]'
```

**目标类型**：裸 token 用 `--type`（默认 docx）；URL 按路径自动识别（docx/doc/sheets/slides/base/file/wiki）。
所有 `text` 元素合计不超过 10000 字符（服务端按合计计数，拆成多个元素不能绕过，超限返回不透明的 1069302），CLI 本地预检。

**reply_elements 元素类型**：
- `text` — 纯文本
- `mention_user` — 提及用户（传 `mention_user` 或 `text` 字段作为 open_id）
- `link` — 链接（传 `link` 或 `text` 字段作为 URL）

**文档输入格式**：
- `docx` token（直接传）
- `docx` URL（`https://xxx.feishu.cn/docx/xxx`）
- `doc` URL（旧版文档）
- `wiki` URL（自动解析到真实 obj_token + obj_type）

### 7. 通用异步任务查询

```bash
# 查询导入任务
feishu-cli drive task-result --scenario import --ticket abcxxx

# 查询导出任务（需要额外传 file-token 作为原始文档 token）
feishu-cli drive task-result --scenario export --ticket abcxxx --file-token docxxxx

# 查询 folder move / file delete 等通用任务
feishu-cli drive task-result --scenario task_check --task-id taskxxx

# wiki 异步任务（wiki move-docs / move-to-drive / delete-space / delete 超时提示的续查命令）
feishu-cli drive task-result --scenario wiki_move --task-id taskxxx --as user
feishu-cli drive task-result --scenario wiki_move_to_drive --task-id taskxxx --as user
feishu-cli drive task-result --scenario wiki_delete_space --task-id taskxxx --as user
feishu-cli drive task-result --scenario wiki_delete_node --task-id taskxxx --as bot
```

七种 scenario：`import` / `export` / `task_check` / `wiki_move` / `wiki_move_to_drive` / `wiki_delete_space` / `wiki_delete_node`。
输出统一带 `ready` / `failed` / `pending`；`task_check` 的失败终态同时识别 `failed` 与删除任务返回的 `fail`。
续查时请带上原命令输出的 `--as`，避免以另一身份查询导致"任务不存在/无权限"。

### 8. 本地 ↔ 云盘单向镜像（pull/push/status）

把云盘文件夹与本地目录做单向镜像，含 SHA-256 内容比对和 `--delete-* --yes` 双确认安全开关。**只镜像 type=file 条目**，docx/sheet/bitable/mindnote/slides/shortcut 等在线文档不参与（没有等价本地二进制）。

```bash
# status：双向 SHA-256 对照，只读，不动文件（只对两边都有的文件流式计算哈希）
feishu-cli drive status --folder-token fldxxx --local-dir ./mirror
feishu-cli drive status --folder-token fldxxx --local-dir ./mirror --quick    # 只比修改时间，不下载远端（近似结果）
# 输出 4 个桶：new_local / new_remote / modified / unchanged，以及 detection=exact|quick

# pull：云盘 → 本地，递归流式下载（无 100MB 上限），下载后本地 mtime 对齐远端 modified_time
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --if-exists smart   # 推荐的增量模式
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --if-exists skip
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --workers 8
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --on-duplicate-remote rename
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --delete-local --yes
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --as bot     # User 缺 drive scope 时

# push：本地 → 云盘，递归上传，自动 create_folder 镜像目录结构
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror              # 默认 --if-exists=skip
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --if-exists overwrite   # 原地覆盖，file_token 不变
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --if-exists smart       # 远端不旧于本地时跳过
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --delete-remote --yes
```

**覆盖与增量**：
- `push --if-exists overwrite` 走 `upload_all`（>20MB 走 `upload_prepare`）**携带 file_token 原地覆盖**：file_token 不变、生成新版本（items 带 `version`），链接/协作者/评论/历史版本全部保留；覆盖失败直接报错，**绝不"先删后传"**；租户未返回 version 时视为失败（改用 `--if-exists skip`）
- `--if-exists smart`：pull 在本地 mtime ≥ 远端 modified_time 时跳过；push 在远端 modified_time ≥ 本地 mtime 时跳过，否则按 overwrite 原地覆盖
- push 上传前复核扫描时的本地快照（大小 + mtime），扫描后被修改的文件标记 `local_file_changed` 失败
- `--on-duplicate-remote`：远端同一路径有多个**文件**时的处理，`fail`（默认，保持旧行为）/ `newest` / `oldest`（pull 还支持 `rename`：最旧的保留原名，其余以 `__lark_<哈希>` 后缀另存）；folder/在线文档与文件重名始终报错

**安全语义**：
- `--local-dir` 走 `filepath.EvalSymlinks` + 限定在 cwd 子树内，防 symlink 越界
- `--delete-local` / `--delete-remote` 必须配 `--yes`，不传 `--yes` 直接拒绝执行
- **带 `--delete-*` 时身份 fail-closed**：若已配置 User Token 但不可用（token.json 未绑定 app_id、app_id 不匹配、刷新失败），命令**直接报错而非降级 Bot 身份**。原因是身份决定「远端有哪些文件」，Bot 视角的远端条目更少、差集更大，`--delete-local` 会把本地文件当作"远端已不存在"而删掉。不带 `--delete-*` 的普通同步仍按 User 优先 + App 兜底（降级时 stderr 告警）。修复办法：`feishu-cli auth token --bind-legacy-app --as user` 或重新 `auth login`
- 上传/下载阶段有失败时**自动跳过 `--delete-*` 阶段**，避免「已删孤儿但部分文件没传成功」的半同步状态
- pull 默认 `--if-exists=overwrite`（保持本地 = 远端），push 默认 `--if-exists=skip`（不动远端已有文件，更安全）
- pull 按**路径段**校验远端名：`report..v2.pdf` 这类文件名正常下载；整段为 `..`/`.`/空段或经本地符号链接目录指向 `--local-dir` 之外的目标会被拒绝（`error_class=unsafe_path`）
- **批量失败分级**：缺 scope（99991672/99991679）、无权限（1061004/403）、限流（99991400）、参数错误（1061002/99992402）、父目录不存在（1061044）、服务端错误等"重跑同一批也不会成功"的错误会**终止整批**（JSON `summary.aborted=true`、`abort_reason`、`not_attempted`，并在 stderr 给分类提示）；items 带 `error_class`/`code`。push 额外把冲突（1061045）、配额、网络错误视为终止
- **1062507 按目录隔离**：push 过程中若上传/建文件夹命中错误码 `1062507`（父目录直接子节点超 1500 上限——该上限是**单个父文件夹**级的），会把该目录标记为已满，其下（含子树）条目全部跳过标记失败，**其余未满目录继续正常镜像**；收尾汇总列出已满目录清单与中文清理建议——先在这些文件夹清理/归档腾出空间，或把本地文件拆分到更细子目录，再重跑

### 9. v2 端点搜索（drive search，扁平 filter）

走 `/open-apis/search/v2/doc_wiki/search` 端点，比 `search docs`（v1）支持更丰富的扁平 filter：

```bash
# 关键字 + 类型过滤 + 排序
feishu-cli drive search --query "季度报告" --doc-types DOCX,SHEET --sort edit_time

# 限定在某些云盘文件夹（与 --space-ids 互斥）
feishu-cli drive search --query "API 设计" --folder-tokens fldxxx,fldyyy

# 限定在知识库 space
feishu-cli drive search --query "RFC" --space-ids spcxxx

# 仅匹配标题（避免正文里命中无关结果）
feishu-cli drive search --query "项目周会" --only-title

# 按创建人
feishu-cli drive search --query "复盘" --creator-ids ou_xxx,ou_yyy

# 按分享群/分享人过滤，或仅搜索评论
feishu-cli drive search --query "决策" --chat-ids oc_xxx --sharer-ids ou_xxx
feishu-cli drive search --query "阻塞" --only-comment

# JSON 输出 + 分页
feishu-cli drive search --query "项目" --page-size 20 -o json
feishu-cli drive search --query "项目" --page-token "<上一页 page_token>"
```

**关键点**：
- `--doc-types` 取值大写：`DOC` / `DOCX` / `SHEET` / `BITABLE` / `MINDNOTE` / `FILE` / `WIKI` / `FOLDER` / `CATALOG` / `SLIDES` / `SHORTCUT`
- `--folder-tokens` 与 `--space-ids` 互斥（doc / wiki 两个范围）
- 标题字段含 `<h>...</h>` 高亮标记，CLI 自动剥离
- 与 `search docs`（v1 `/suite/docs-api/search/object`）共存：v1 走 owner_ids/chat_ids 简单过滤，v2 走 doc_filter+wiki_filter 双路精细过滤

### 10. 密级标签（secure-label）

查看/设置云文档密级标签。**仅支持用户身份**（必需 User Token，`requireUserToken`），需要 `docs:secure_label:*` scope。

```bash
# 查询当前用户可用的密级标签（先拿标签 id，别用显示名）
feishu-cli drive secure-label list --page-size 10 --lang zh
feishu-cli drive secure-label list --output json

# 把某文档设置为指定密级（--label-id 用 list 返回的数字 id）
feishu-cli drive secure-label set doxcnxxxx --type docx --label-id 7217780879644737539
```

**关键点**：
- `list`：底层 `GET /open-apis/drive/v2/my_secure_labels`，`--page-size` 取值 1-10，`--lang` 支持 `zh/en/ja`；返回每个标签的 `id` + 名称
- `set`：底层 `PATCH /open-apis/drive/v2/files/{file_token}/secure_label?type={type}`，请求体 `{"id": label_id}`；`--type` 默认 `docx`，可选 `doc/docx/sheet/file/bitable/mindnote/slides`
- `--label-id` 必须是 list 返回的**数字 id**，不要传显示名（如 `内部(D)`）
- **密级降级需审批**：命中错误码 `1063013` 时，需到文档界面完成密级降级审批后重试，重试 API 不会绕过审批

## 典型工作流

### 工作流 A：大文件分块上传 + 查看进度

```bash
feishu-cli drive upload --file big_video.mp4 --folder-token fldxxx --name "会议录像.mp4"
# 自动走分块，stderr 输出分片进度
# 上传: 会议录像.mp4 (104857600 bytes)
# 分片上传: 文件大小 100.0 MB, 分片大小 4.0 MB, 共 25 个分片
#   分片 1/25 上传完成 (4.0 MB)
#   ...
# file_token 返回后可直接在飞书里访问
```

### 工作流 B：docx 批量导出 markdown

```bash
# 通过 docs_ai fetch 快捷路径，秒出不用等
for doc_id in doc1 doc2 doc3; do
  feishu-cli drive export --token $doc_id --doc-type docx --file-extension markdown --output-dir ./docs
done
```

### 工作流 C：导出长文档（超时 resume）

```bash
# 1. 触发导出
TICKET=$(feishu-cli drive export --token docxxxxx --doc-type docx --file-extension pdf -o json | jq -r '.ticket // empty')

# 2. 如果超时，输出会带 next_command
#    手动或脚本化接力：
feishu-cli drive task-result --scenario export --ticket $TICKET --file-token docxxxxx

# 3. 任务就绪后下载产物
feishu-cli drive export-download --file-token boxxxx --output-dir ./exports
```

### 工作流 D：wiki 链接一键评论

```bash
# 不需要先解析 wiki 到 docx，drive add-comment 自动反查
feishu-cli drive add-comment \
  --doc "https://xxx.feishu.cn/wiki/nodxxxxx" \
  --content '[
    {"type":"text","text":"收到，已处理 "},
    {"type":"mention_user","mention_user":"ou_abc123"}
  ]'
```

### 工作流 E：本地 docx 导入为飞书文档

```bash
# drive import 走临时媒体（不污染云盘）
feishu-cli drive import --file report.docx --type docx --folder-token fldxxx

# XLSX 超过 20MB 自动分片，仍可保留普通 Sheet 类型（上限 800MB）
feishu-cli drive import --file big_sheet.xlsx --type sheet --folder-token fldxxx
```

不要仅因文件超过 20MB 就改成 Bitable；目标类型以用户需求为准。CSV 的上限另计：Sheet 20MB、Bitable 100MB；更大的 CSV 若需保留 Sheet，应先转换为 XLSX。完整格式矩阵见上方「文档导入」。

### 工作流 F：申请文档权限（apply-permission）

碰到「没有权限查看此文档」时，不需要去飞书 IM 私聊文档所有者，直接在终端申请：

```bash
# 1. 先用 inspect 确认 token 类型（可选；apply-permission 也支持 URL 直接传）
feishu-cli drive inspect --url "https://xxx.feishu.cn/docx/doxcnxxx"
# → 输出 type=docx, title=..., token=doxcnxxx

# 2. 申请只读权限（带申请说明，会出现在发给所有者的审批卡片上）
feishu-cli drive apply-permission \
  --token "https://xxx.feishu.cn/docx/doxcnxxx" \
  --perm view \
  --remark "调研 RFC，需要查看背景设计"

# 3. 申请编辑权限（同一接口，--perm edit）
feishu-cli drive apply-permission --token doxcnxxx --type docx --perm edit --remark "..."

# 4. 预览即将发出的请求（不实际申请）
feishu-cli drive apply-permission --token <url> --perm view --dry-run
```

> ⚠️ 这是「埋藏 API」（飞书文档站未收录但服务端可用，已实测），必需 User Token + `docs:permission.member:apply` scope（或任一大权限如 `drive:drive`）。
>
> 业务错误（含 HTTP 200 + code≠0）一律以非零退出码失败：`1063006` = 同一用户对同一文档每天最多申请 5 次，等次日额度重置；
> `1063007` = 该文档不接受权限申请，核对目标与申请的权限，或直接联系所有者。

### 工作流 G：拿到 URL → 一行命令解析出 token / 标题 / 类型（inspect）

写脚本或 Agent 编排时常需要从 URL 反查文档元信息，`inspect` 是最快路径：

```bash
# docx URL → 输出 type=docx + 标题 + 裸 token + canonical URL
feishu-cli drive inspect --url "https://xxx.feishu.cn/docx/doxcnxxx"

# wiki URL → 自动展开到底层文档（自动调 wiki node_by_token 拆 obj_token + obj_type）
feishu-cli drive inspect --url "https://xxx.feishu.cn/wiki/wikcnxxx"
# → 输出 type=docx, token=<真实 docx token>

# 裸 token：未传 --type 时自动调 GET /drive/v2/files/query_by_token 识别类型（Bot/User 均可用，
# wiki node_token 自动识别并展开；JSON 带 detected_by=query_by_token，节点在回收站/已删除时带 token_status）
feishu-cli drive inspect --url doxcnxxx

# 裸 token + 显式 type → 跳过识别，直接查标题
feishu-cli drive inspect --url doxcnxxx --type docx

# JSON 输出（脚本/Agent 友好）
TOKEN=$(feishu-cli drive inspect --url <url> -o json | jq -r '.token')
TITLE=$(feishu-cli drive inspect --url <url> -o json | jq -r '.title')

# 与其他命令串起来：解析 wiki → 导出 markdown
DOC_TOKEN=$(feishu-cli drive inspect --url "https://xxx.feishu.cn/wiki/wikcnxxx" -o json | jq -r '.token')
feishu-cli drive export --token $DOC_TOKEN --doc-type docx --file-extension markdown --output-dir ./out
```

> 与其他命令的区别：`inspect` 是**只读、不强制 User Token**（User 优先 + App 兜底），未登录也能查公开/Bot 可见的文档元信息；登录后会用 User Token 看到完整权限范围。

## 与老命令的对照

| 老命令 | 新 drive 命令 | 差异 |
|---|---|---|
| `file upload` | `drive upload` | drive 支持 User Token + 分块 + 每片重试 |
| `file download` | `drive download` | 两者都是流式 + 分片重试 + 原子写；drive 额外支持 URL/wiki 解包、在线文档识别、默认文件名、`--overwrite`、`--as` |
| `file move` | `drive move` | drive 文件夹移动自动轮询 task_check |
| `doc export-file --type pdf` | `drive export --doc-type docx --file-extension pdf` | drive 增加 markdown 快捷路径 + sub-id + resume |
| `doc import-file --type docx` | `drive import --type docx` | drive 走 `/medias/upload_all`（不留中间文件） |
| `comment add --type docx` | `drive add-comment --doc <url>` | drive 支持富文本 + wiki 解析 + 局部评论 |

**老命令不会被删除**，仍然可以用（走 App Token 简单场景），但新能力只在 `drive` 命令组里。

## 权限要求

| 命令 | Token 策略 | 所需 scope |
|---|---|---|
| `drive upload` | 默认必需 User Token；`--as bot\|user\|auto` 可切换（Bot 新建后自动给当前用户授 full_access） | `drive:file:upload` |
| `drive download` | 默认必需 User Token；`--as bot\|user\|auto` 可切换 | `drive:file:download`（query_by_token 识别另需 `drive:drive.metadata:readonly`，缺失只告警） |
| `drive update-title` | `--as bot\|user\|auto`（默认 auto） | `drive:file:upload` 或对应类型的编辑 scope；`--type file` 的扩展名保护另需 `drive:drive.metadata:readonly` |
| `drive version-history` / `version-get` | `--as bot\|user\|auto`（默认 auto） | `drive:file:download` |
| `drive export` | `--as bot\|user\|auto`（默认 auto；已配置 User 刷新失败 fail-closed） | `docs:document:export`、`drive:drive.metadata:readonly`（导出 markdown 还需 `docs:document.content:read`） |
| `drive export-download` | `--as bot\|user\|auto`（默认 auto） | `drive:file:download` |
| `drive import` | `--as bot\|user\|auto`（默认 auto） | `docs:document:import`、`drive:file:upload` |
| `drive move` | `--as bot\|user\|auto`（默认 auto） | `drive:file:write` [^1] |
| `drive add-comment` | 必需 User Token | `docs:document.comment:create`、`docs:document.comment:write_only`；wiki URL 还需 `wiki:node:read`；docx 局部评论还需 `docx:document:readonly` |
| `drive task-result` | `--as bot\|user\|auto`（默认 auto） | `drive:drive.metadata:readonly`（具体依 scenario：`import` 还需 `docs:document:import`；`export` 还需 `docs:document:export`；`wiki_*` 需 `wiki:space:read` 或 `wiki:wiki`） |
| `drive pull` / `status` | 不传 `--as`：User 优先 + App 兜底；带 `--delete-local` 时 fail-closed（不降级 Bot）；`--as bot\|user\|auto` 显式指定 | `drive:drive.metadata:readonly`、`drive:file:download`（`status --quick` 不需要）；带 `--delete-local` 还需本地删除权限 |
| `drive push` | 不传 `--as`：User 优先 + App 兜底；带 `--delete-remote` 时 fail-closed（不降级 Bot）；`--as bot\|user\|auto` 显式指定 | `drive:drive.metadata:readonly`、`drive:file:upload`、`space:folder:create`；带 `--delete-remote` 还需 `space:document:delete` |
| `drive search` | 必需 User Token | `search:docs:read` |
| `drive upload --file-token`（覆盖） | 默认必需 User Token；`--as` 可切换 | `drive:file:upload`（覆盖不改变已有权限设置） |
| `drive secure-label list` | 必需 User Token | `docs:secure_label:readonly` |
| `drive secure-label set` | 必需 User Token | `docs:secure_label:write_only` |
| `drive inspect` | User 优先 + App 兜底（不强制 User Token） | `drive:drive.metadata:readonly`；wiki URL 还需 `wiki:node:read` |
| `drive apply-permission` | 必需 User Token | `docs:permission.member:apply`（或任一大权限：`drive:drive` / `docs:doc` / `docx:document` 等） |

[^1]: `drive:file:write` 是 CLI help 与代码当前声明的 scope；飞书部分 API 文档历史上也提到 `space:document:move`，**具体以飞书最新 OpenAPI 文档为准**。两者覆盖同一动作，新版应用直接申请 `drive:file:write`（更通用，写类操作通用 scope）。

## 注意事项

- **Token 策略**：以「权限要求」表为唯一权威（`download/upload` 默认必需 User Token、可 `--as` 切换；`import/export/export-download/move/update-title/version-*/task-result` 走 `--as`，默认 auto 且刷新失败 fail-closed；`pull/push/status/inspect` 未登录可回落 App Token）。
- **SSRF 防护**：下载 URL 会被校验，拒绝 localhost / 回环 IP / 内网段 / 链路本地
- **重定向策略**：下载 HTTP 重定向最多 5 次，禁止 HTTPS → HTTP 降级
- **大文件分块阈值**：固定 20MB，超过自动切分片
- **导出有界轮询**：10 次 × 5 秒（总共 50 秒），超时**不报错**而是返回 `next_command`
- **导入有界轮询**：30 次 × 2 秒（总共 60 秒），超时同上
- **文件夹移动轮询**：30 次 × 2 秒
- **轮询容错**：查询瞬时失败继续轮询；限流立即停止并在错误中给出带 `--as` 的续查命令；全部查询失败时报错（任务已创建，续查即可）
- **格式特定大小限制**（import）：按源扩展名，见上方矩阵（不再是笼统的 docx/sheet 20MB）
- **drive move 省略 --folder-token**：先取真实根目录 token，不会把空字符串交给 move API
- **add-comment 的 wiki 解析**：支持 obj_type 为 docx/doc/sheet/slides/bitable/file 的 wiki 节点；mindnote 等其他类型会报错
- **局部评论**：docx 用 `--block-id <block_id>`；sheet/slides/bitable 必须带对应格式的 `--block-id`；doc（旧版文档）与 file 只支持全文评论
- **文件名规则**：
  - **`drive download`**：`--output` 省略或为目录时按 `Content-Disposition` → 云盘标题 → `file_token` 依次决定文件名；要自定义名字请显式传文件路径
  - **`minutes download`**（参见 feishu-cli-meetings）：从响应头按 `Content-Disposition > filename* > Content-Type 推导扩展名 > {token}.media` 优先级解析
