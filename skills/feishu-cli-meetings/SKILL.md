---
name: feishu-cli-meetings
description: >-
  查询飞书历史视频会议、纪要、AI 摘要、逐字稿和录制，按 minute token 读取或下载妙记，操作会议机器人入会/离会及查询会议事件。创建日程、找共同空闲时间和预订会议室使用 feishu-cli-work。
compatibility: Requires feishu-cli v1.42.0+ and network access for Feishu API calls.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Read Write
---

# 飞书会议与妙记

本 Skill 只有一个工作流 `references/workflows/vc/workflow.md`，按下表定位到其中的章节后执行。
将该工作流中的 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按 `workflow.md`
所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 命令 | 工作流章节 |
|---|---|---|
| 按主题/时间/参会人搜历史会议，会议号换 meeting_id，一次拿 note_id 与 minute_token | `vc search`、`vc detail`、`vc recording` | 命令：搜索与定位会议 |
| 会议纪要、纪要文档、逐字稿（普通纪要读 verbatim 文档，统一纪要导出统一逐字稿） | `vc notes`、`vc note detail/transcript`、`doc export` | 决策规则、命令：纪要与逐字稿 |
| 妙记搜索、AI 摘要/待办/章节/关键词、妙记逐字稿、下载音视频、申请妙记权限 | `minutes search/get/download/apply-permission` | 命令：妙记 |
| 正在开的会议、会中事件 | `vc meeting list-active`、`vc bot meeting-events` | 命令：进行中的会议与会中事件 |
| 会议机器人入会/离会 | `vc bot meeting-join/meeting-leave` | 命令：会议机器人入会/离会 |
| 日历日程直达纪要与妙记 | `calendar agenda` → `vc notes --calendar-event-ids` | 典型工作流 B |

## 执行规则

1. 身份：`vc search/detail/recording/notes`、`vc note detail`、`vc meeting list-active` 与 minutes 命令默认 User，
   可 `--as bot|auto`（Bot 需应用开通 scope，且通常无权读取用户的会议和妙记）；`vc note transcript` 只支持 User；
   `meeting-join/leave` 只支持 Bot（传 `--user-access-token` 报错 exit 2）；`meeting-events` 的 `--as` 必须与
   `meeting_id` 来源一致。取得 ID 时用的身份要沿用到后续命令，命令不支持时说明限制，不要擅自换身份。
2. 标识不能混用：9 位数字是会议号，只能给 `vc detail <会议号>`、`vc search --query` 和 `meeting-join --meeting-number`；
   `meeting-events` / `meeting-leave` 要长数字 `meeting_id`。
3. 读逐字稿先看纪要类型：`normal` 用 `doc export <verbatim_doc>`，`unified` 用 `vc note transcript <note_id>`，
   只有妙记时用 `minutes get <minute_token> --transcript`（写文件）。
4. `vc notes --with-artifacts/--download-transcript` 只对 `--minute-tokens` 生效，`--meeting-ids` 会静默忽略；
   只要摘要、待办等时用 `minutes get --summary --todo ...`，不要拉整份产物。
5. 批量命令部分失败时退出码仍为 0，逐项检查 `ok`、`error`、`hint`、`transcript_path` 与 `artifacts_error`。
6. `minutes apply-permission` 会通知妙记所有者，机器人入会/离会对参会人可见：执行前征得用户同意，验证参数用 `--dry-run`。
7. 会议结束后不要再用 `meeting-events`，改读 `vc detail` / `vc notes` 的会后产物；会中事件先 `vc meeting list-active` 拿 `meeting_id`。

User 路径业务命令前预检，例如搜索会议并读取妙记逐字稿（`auth check` 只检查 User Token；Bot 路径需确认应用侧已开通 scope，不能用它替代）：

```bash
feishu-cli auth check --scope "vc:meeting.search:read vc:note:read minutes:minutes:readonly minutes:minutes.artifacts:read"
```

删除、覆盖类命令返回退出码 10 时，向用户确认目标与影响后追加全局 `--yes` 重跑，不要自行添加。身份或 scope 报错（如 99991663/99991668/99991672/99991679）读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md`；判断成败、编写脚本或处理确认门禁读取 `../feishu-cli-platform/references/workflows/auth/references/agent-contract.md`。
