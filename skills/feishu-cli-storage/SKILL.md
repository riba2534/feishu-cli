---
name: feishu-cli-storage
description: >-
  飞书云空间统一入口，覆盖 Drive 增强上传下载与异步导入导出、基础 file/media 操作、
  wiki 知识库节点和空间管理、文档评论、协作者和公开权限。用户提到云盘文件、文件夹、
  大文件分块或断点续传、异步任务、目录镜像、知识库、wiki、素材、评论、共享权限、
  协作者、公开链接、分享密码、转移所有权或权限申请时必须使用本 Skill。
  Wiki 仅在管理空间、节点结构或成员时属于本 Skill；明确禁止用它读取或总结 Wiki/文档正文，
  正文内容使用 feishu-cli-docs。会议录制、妙记和逐字稿使用 feishu-cli-meetings；搜索使用
  feishu-cli-platform。
argument-hint: <drive|file|media|wiki|comment|perm> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Bash(jq:*), Bash(python3:*), Read, Write
---

# 飞书云空间

加载工作流后，将其中 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| 大文件、断点续传、异步导入导出、镜像、Drive 搜索 | `references/workflows/drive/workflow.md` |
| 基础 file CRUD、版本、元数据、素材上传下载 | `references/workflows/file-media/workflow.md` |
| wiki 空间、节点、成员、递归导出 | `references/workflows/wiki/workflow.md` |
| 评论、回复、解决和取消解决 | `references/workflows/comment/workflow.md` |
| 协作者、公开权限、密码、转移所有权 | `references/workflows/perm/workflow.md` |

## 执行规则

1. 基础文件操作优先 file/media；需要分块、resume、镜像或异步任务时使用 drive。
2. 删除、移动、覆盖、转移所有权和删除知识空间属于高风险操作，先确认目标并使用命令提供的确认参数。
3. 个人文档的评论和权限常需 User Token；应用文档可使用 App Token。以工作流和实际帮助为准。
4. wiki node token 与普通 document/file token 不可混用。
