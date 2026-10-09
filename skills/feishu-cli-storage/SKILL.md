---
name: feishu-cli-storage
description: >-
  飞书云空间统一入口，覆盖 Drive 增强上传下载与异步导入导出、基础 file/media 操作、
  wiki 知识库节点和空间管理、文档评论、协作者和公开权限。用户提到云盘文件、文件夹、
  大文件分块或断点续传、异步任务、目录镜像、知识库、wiki、素材、评论、共享权限、
  协作者、公开链接、分享密码、转移所有权或权限申请时必须使用本 Skill。
  Wiki 仅在管理空间、节点结构或成员时属于本 Skill；明确禁止用它读取或总结 Wiki/文档正文，
  正文内容使用 feishu-cli-docs。会议录制、妙记和逐字稿使用 feishu-cli-meetings；Drive 范围搜索属于本 Skill；全局文档/消息/应用搜索使用 feishu-cli-platform。
compatibility: Requires feishu-cli v1.42.0+ and network access for Feishu API calls.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Bash(jq:*) Bash(python3:*) Read Write
---

# 飞书云空间

加载工作流后，将其中 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| 大文件分块上传、原地覆盖上传、流式/断点下载、URL 或 wiki 包装文件下载 | `references/workflows/drive/workflow.md` |
| 解析 URL/裸 token 的类型与标题（`drive inspect`）、重命名文件/文件夹/文档/wiki 节点（`drive update-title`） | `references/workflows/drive/workflow.md` |
| 上传文件（type=file）的版本历史与按版本下载（`drive version-history/version-get`） | `references/workflows/drive/workflow.md` |
| 异步导入导出与续查（`drive import/export/export-download/task-result`）、文件夹移动、目录镜像（pull/push/status） | `references/workflows/drive/workflow.md` |
| 富文本/局部/单元格/记录评论（`drive add-comment`）、申请文档权限、密级标签、Drive 范围搜索 | `references/workflows/drive/workflow.md` |
| 文件夹浏览与分页、基础 file CRUD、快捷方式、元数据/统计、容量（`file quota`）、文档素材上传下载 | `references/workflows/file-media/workflow.md` |
| 在线文档命名版本（`file version list/create/get/delete`）与上传文件回滚（`file version revert`） | `references/workflows/file-media/workflow.md` |
| wiki 空间、节点解析（obj_token/URL）、创建/移动/复制/删除、云盘与知识库互移、成员、递归导出 | `references/workflows/wiki/workflow.md` |
| 评论列出、读取、回复、编辑回复、表情回应、解决和取消解决 | `references/workflows/comment/workflow.md` |
| 协作者、公开权限、分享密码、转移所有权、权限检查、Bot 创建后的自动授权（`permission_grant`） | `references/workflows/perm/workflow.md` |

## 执行规则

1. 基础文件操作优先 file/media；需要分块、断点、URL/wiki 解包、原地覆盖、版本历史、镜像或异步任务时使用 drive。
   上传文件的历史版本（`drive version-history`，长数字 version）与在线文档命名版本（`file version`）不要混用。
2. 删除、移动、覆盖、`--delete-local/--delete-remote`、转移所有权和删除知识空间属于高风险操作：先确认目标，再带命令的确认参数；
   非交互环境未带 `--yes`（或命令级 `--force`）时以退出码 10 拒绝执行，不代表成功。`wiki delete` 默认级联删除整棵子树。
3. 身份：perm 全组默认 Bot，操作用户自己的文档加 `--as user`（不读 `FEISHU_USER_ACCESS_TOKEN`），Bot 报 1063002/1063004 时改用 User；
   file/wiki 写类默认 Bot，只认显式 `--user-access-token`，Bot 移动、删除用户自有文件会失败；comment 读与 add/resolve 默认 User 优先，
   `comment reply add/update/react/delete` 默认 Bot 且必须与回复作者同身份；`drive add-comment/search/secure-label/apply-permission`、
   `file quota`、`wiki space-create` 必须 User；`drive upload/download` 默认要求 User，可 `--as bot`。
4. 读类或镜像命令遇 99991679（User Token 缺 drive 读 scope）时，资源对应用可见可显式 `--as bot`，否则补授权；带 `--delete-*` 时
   已配置但不可用的 User Token 会 fail-closed，不降级 Bot。异步任务续查沿用原命令输出里的 `--as`。
5. wiki `get/update/move/export/export-tree/delete` 接受 node_token、已挂载文档的 obj_token 或 URL；`move-to-drive`、`node-copy`、
   `nodes --parent`、`drive update-title --type wiki`、`perm --doc-type wiki` 只接受 node_token，先 `wiki get` 换出。
6. Bot 身份新建文件、文件夹、文档或 wiki 节点后会自动给当前 CLI 登录用户授 `full_access`（JSON `permission_grant`），
   交付给其他人仍需 `perm add`。

遇到 Token、身份或 scope 报错（如 99991663/99991668/99991672/99991679）时，读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md` 确认应使用的身份与预检方式，排错表见 `../feishu-cli-platform/references/workflows/auth/workflow.md`。
