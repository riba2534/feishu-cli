# 飞书云盘增强（Drive）

`drive` 命令组提供与老 `file` / `media` / `comment add` 命令**并存**的增强能力：
分块上传、markdown 快捷导出、异步任务 resume、富文本评论、wiki 链接解析。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 目录

- [前置条件](#前置条件)
- [命令速查](#命令速查)
- [典型工作流](#典型工作流)
- [与基础命令的对照](#与老命令的对照)
- [权限要求](#权限要求)
- [注意事项](#注意事项)

## 前置条件

- **认证**：
  - **必需 User Token**（`requireUserToken`，未登录直接报错）：`upload`/`download`/`export`/`export-download`/`import`/`move`/`add-comment`/`task-result`/`search`/`apply-permission`，先 `feishu-cli auth login`。
  - **登录后 User 优先 + App 兜底**（`resolveOptionalUserTokenWithFallback`，未登录不报错）：`pull`/`push`/`status`/`inspect`。已 `auth login` 自动用 User Token；未登录回落 App Token（注意 Bot 通常看不到你云盘里的私人文件）。
- **预检**：`feishu-cli auth check --scope "drive:file:upload"` 可验证 scope

## 命令速查

### 0. 元数据 / 权限申请（v1.29+ 新增 ⭐）

```bash
# 解析任意文档 URL → 输出 type/title/canonical token（自动展开 wiki）
feishu-cli drive inspect --url "https://xxx.feishu.cn/docx/doxcnxxx"
feishu-cli drive inspect --url "https://xxx.feishu.cn/wiki/wikcnxxx"   # 自动 wiki get_node
feishu-cli drive inspect --url doxcnxxx --type docx -o json            # 裸 token + JSON

# 向文档所有者申请权限（埋藏 API，飞书文档站未收录但服务端可用）
# 必需 User Token + docs:permission.member:apply scope（或 drive:drive 等任一大权限）
feishu-cli drive apply-permission --token "<url 或裸 token>" --perm view --remark "申请理由"
feishu-cli drive apply-permission --token doxcnxxx --type docx --perm edit --remark "..."
feishu-cli drive apply-permission --token <url> --perm view --dry-run    # 预览请求
```

详见 [`embedded-api-discovery.md`](../../../../feishu-cli-platform/references/workflows/api/references/embedded-api-discovery.md)（埋藏 API 调研方法论）。

### 1. 上传 / 下载

```bash
# 上传（>20MB 自动走 3 段式分块 upload_prepare/upload_part/upload_finish）
feishu-cli drive upload --file /tmp/report.pdf
feishu-cli drive upload --file /tmp/big.zip --folder-token fldxxx --name "年度报告.zip"

# 原地覆盖：把已有文件更新为新版本（file_token 不变、刷新 version/size，权限设置保持不变）
feishu-cli drive upload --file /tmp/report.pdf --file-token boxcnxxxx

# 下载（流式 + 路径校验 + --overwrite + --timeout，大文件自动 Range 分片兜底）
feishu-cli drive download --file-token boxcnxxxx --output ./report.pdf
feishu-cli drive download --file-token boxcnxxxx --output ./downloads/ --overwrite
feishu-cli drive download --file-token boxcnxxxx --output ./big.zip --timeout 30m
feishu-cli drive download --file-token boxcnxxxx --output ./report.pdf --output-format json
```

**关键点**：
- `drive upload` 分块上传每片独立重试 3 次，使用 `io.SectionReader` 外层只打开文件一次
- `drive upload --file-token`：把本地文件覆盖为该文件的新版本，file_token 不变；**不会改变文件已有权限设置**（协作者、公开范围等保持原样）。仅支持 ≤ 20MB（单次上传上限），与 `--folder-token` 互斥
- `drive download` 的 `--output` 可以是文件路径（直接用）或目录（**文件名用 `file_token` 本身**，暂不从响应头解析）
- `drive download` 使用用户身份下载时，如果服务端返回大文件限制，会自动使用 HTTP Range 分片下载并合并
- 如果需要自定义文件名，请显式传完整路径：`--output ./downloads/report.pdf`

### 2. 文档导出（含 markdown 快捷路径）

```bash
# docx → markdown：走 /docs/v1/content 快捷路径，立即返回（不跑异步 export_tasks）
feishu-cli drive export --token docxxxx --doc-type docx --file-extension markdown --output-dir ./exports

# docx → pdf：走异步 export_tasks，有界轮询 10×5s，超时返回 next_command
feishu-cli drive export --token docxxxx --doc-type docx --file-extension pdf --output-dir ./exports

# sheet → csv 指定 sheet_id
feishu-cli drive export --token sheetxxxx --doc-type sheet --file-extension csv --sub-id sheet_1

# bitable → csv 指定 table_id
feishu-cli drive export --token basexxxx --doc-type bitable --file-extension csv --sub-id tblxxxx
```

**支持的格式**：
- `--doc-type`: `doc` / `docx` / `sheet` / `bitable`
- `--file-extension`: `docx` / `pdf` / `xlsx` / `csv` / `markdown`（markdown 仅 docx 支持）

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
# 本地文件 → 云文档（docx / sheet / bitable）
feishu-cli drive import --file report.docx --type docx
feishu-cli drive import --file data.xlsx --type sheet --folder-token fldxxx
feishu-cli drive import --file bigsheet.csv --type bitable --folder-token fldxxx

# 注意：docx/sheet 导入上限 20MB，bitable 上限 100MB
```

**关键技术点**：
- 走 **官方 `/medias/upload_all` 端点**（`parent_type=ccm_import_open` + `extra`），**不在用户云盘留下中间文件**（这是和老 `doc import-file` 的核心区别）
- 格式特定大小限制：docx 20MB / sheet 20MB / bitable 100MB。docx/sheet 超过 20MB 会直接报错，不会自动绕过格式限制。
- 有界轮询 30×2s，超时返回 `next_command`

### 5. 移动（文件夹自动轮询）

```bash
# 文件移动（同步，立即返回）
feishu-cli drive move --file-token boxxxx --type docx --folder-token fldxxx

# 文件夹移动（异步，自动轮询 task_check 30×2s）
feishu-cli drive move --file-token fldxxx --type folder --folder-token fldyyy
```

**关键点**：
- **文件夹移动自动轮询**，不再是"发出去就不管了"
- 超时会返回 `task_id`，可用 `drive task-result --scenario task_check` 接力

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
```

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

# 查询 folder move 等通用任务
feishu-cli drive task-result --scenario task_check --task-id taskxxx
```

三种 scenario：`import` / `export` / `task_check`。用于 `drive export` / `drive import` / `drive move` 超时后的接力完成。

### 8. 本地 ↔ 云盘单向镜像（pull/push/status）

把云盘文件夹与本地目录做单向镜像，含 SHA-256 内容比对和 `--delete-* --yes` 双确认安全开关。**只镜像 type=file 条目**，docx/sheet/bitable/mindnote/slides/shortcut 等在线文档不参与（没有等价本地二进制）。

```bash
# status：双向 SHA-256 对照，只读，不动文件
feishu-cli drive status --folder-token fldxxx --local-dir ./mirror
# 输出 4 个桶：new_local / new_remote / modified / unchanged

# pull：云盘 → 本地，递归下载
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --if-exists skip
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --workers 8
feishu-cli drive pull --folder-token fldxxx --local-dir ./mirror --delete-local --yes

# push：本地 → 云盘，递归上传，自动 create_folder 镜像目录结构
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror              # 默认 --if-exists=skip
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --if-exists overwrite
feishu-cli drive push --folder-token fldxxx --local-dir ./mirror --delete-remote --yes
```

**安全语义**：
- `--local-dir` 走 `filepath.EvalSymlinks` + 限定在 cwd 子树内，防 symlink 越界
- `--delete-local` / `--delete-remote` 必须配 `--yes`，不传 `--yes` 直接拒绝执行
- 上传/下载阶段有失败时**自动跳过 `--delete-*` 阶段**，避免「已删孤儿但部分文件没传成功」的半同步状态
- pull 默认 `--if-exists=overwrite`（保持本地 = 远端），push 默认 `--if-exists=skip`（不动远端已有文件，更安全）
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
# 通过 /docs/v1/content 快捷路径，秒出不用等
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

# 20MB 以上表格文件请导入为 bitable（上限 100MB）
feishu-cli drive import --file big_sheet.xlsx --type bitable --folder-token fldxxx
```

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

### 工作流 G：拿到 URL → 一行命令解析出 token / 标题 / 类型（inspect）

写脚本或 Agent 编排时常需要从 URL 反查文档元信息，`inspect` 是最快路径：

```bash
# docx URL → 输出 type=docx + 标题 + 裸 token + canonical URL
feishu-cli drive inspect --url "https://xxx.feishu.cn/docx/doxcnxxx"

# wiki URL → 自动展开到底层文档（自动调 wiki get_node 拆 obj_token + obj_type）
feishu-cli drive inspect --url "https://xxx.feishu.cn/wiki/wikcnxxx"
# → 输出 type=docx, token=<真实 docx token>

# 裸 token + 显式 type → 拿标题做权限/分类
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
| `file download` | `drive download` | drive 支持 User Token + `--overwrite` + `--timeout` + 路径校验 |
| `file move` | `drive move` | drive 文件夹移动自动轮询 task_check |
| `doc export-file --type pdf` | `drive export --doc-type docx --file-extension pdf` | drive 增加 markdown 快捷路径 + sub-id + resume |
| `doc import-file --type docx` | `drive import --type docx` | drive 走 `/medias/upload_all`（不留中间文件） |
| `comment add --type docx` | `drive add-comment --doc <url>` | drive 支持富文本 + wiki 解析 + 局部评论 |

**老命令不会被删除**，仍然可以用（走 App Token 简单场景），但新能力只在 `drive` 命令组里。

## 权限要求

| 命令 | Token 策略 | 所需 scope |
|---|---|---|
| `drive upload` | 必需 User Token | `drive:file:upload` |
| `drive download` | 必需 User Token | `drive:file:download` |
| `drive export` | 必需 User Token | `docs:document:export`、`drive:drive.metadata:readonly`（导出 markdown 还需 `docs:document.content:read`） |
| `drive export-download` | 必需 User Token | `drive:file:download` |
| `drive import` | 必需 User Token | `docs:document:import`、`drive:file:upload` |
| `drive move` | 必需 User Token | `drive:file:write` [^1] |
| `drive add-comment` | 必需 User Token | `docs:document.comment:create`、`docs:document.comment:write_only`；wiki URL 还需 `wiki:node:read`；docx 局部评论还需 `docx:document:readonly` |
| `drive task-result` | 必需 User Token | `drive:drive.metadata:readonly`（具体依 scenario：`import` 还需 `docs:document:import`；`export` 还需 `docs:document:export`） |
| `drive pull` / `status` | User 优先 + App 兜底 | `drive:drive.metadata:readonly`、`drive:file:download` |
| `drive push` | User 优先 + App 兜底 | `drive:drive.metadata:readonly`、`drive:file:upload`、`space:folder:create`；带 `--delete-remote` 还需 `drive:file:delete` |
| `drive search` | 必需 User Token | `search:docs:read` |
| `drive upload --file-token`（覆盖） | 必需 User Token | `drive:file:upload`（覆盖不改变已有权限设置） |
| `drive secure-label list` | 必需 User Token | `docs:secure_label:readonly` |
| `drive secure-label set` | 必需 User Token | `docs:secure_label:write_only` |
| `drive inspect` | User 优先 + App 兜底（不强制 User Token） | `drive:drive.metadata:readonly`；wiki URL 还需 `wiki:node:read` |
| `drive apply-permission` | 必需 User Token | `docs:permission.member:apply`（或任一大权限：`drive:drive` / `docs:doc` / `docx:document` 等） |

[^1]: `drive:file:write` 是 CLI help 与代码当前声明的 scope；飞书部分 API 文档历史上也提到 `space:document:move`，**具体以飞书最新 OpenAPI 文档为准**。两者覆盖同一动作，新版应用直接申请 `drive:file:write`（更通用，写类操作通用 scope）。

## 注意事项

- **Token 策略**：`upload/download/export/export-download/import/move/add-comment/task-result/search/apply-permission` 必需 User Token；`pull/push/status/inspect` 登录后默认 User Token，未登录回落 App Token。
- **SSRF 防护**：下载 URL 会被校验，拒绝 localhost / 回环 IP / 内网段 / 链路本地
- **重定向策略**：下载 HTTP 重定向最多 5 次，禁止 HTTPS → HTTP 降级
- **大文件分块阈值**：固定 20MB，超过自动切分片
- **导出有界轮询**：10 次 × 5 秒（总共 50 秒），超时**不报错**而是返回 `next_command`
- **导入有界轮询**：30 次 × 2 秒（总共 60 秒），超时同上
- **文件夹移动轮询**：30 次 × 2 秒
- **格式特定大小限制**（import）：docx 20MB / sheet 20MB / bitable 100MB
- **add-comment 的 wiki 解析**：只支持 obj_type 为 `docx` 或 `doc` 的 wiki 节点；其他类型（sheet/bitable/mindnote 等）会报错
- **局部评论**：仅 docx 支持 `--block-id` 锚点，doc（旧版文档）不支持
- **文件名规则**：
  - **`drive download`**：`--output` 为目录时使用 `file_token` 作为文件名（不从响应头解析）；要自定义名字请显式传文件路径
  - **`minutes download`**（参见 feishu-cli-meetings）：从响应头按 `Content-Disposition > filename* > Content-Type 推导扩展名 > {token}.media` 优先级解析
