---
name: feishu-cli-meetings
description: >-
  查询飞书历史视频会议、纪要、AI 摘要、逐字稿和录制，按 minute token 读取或下载妙记，操作会议机器人入会/离会及查询会议事件。创建日程、找共同空闲时间和预订会议室使用 feishu-cli-work。
compatibility: Requires feishu-cli v1.41.0+ and network access for Feishu API calls.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Read Write
---

# 飞书会议与妙记

读取 `references/workflows/vc/workflow.md` 后执行。
将该工作流中的 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按 `workflow.md`
所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 身份边界

- `vc search/notes/recording/detail`、`vc note detail/transcript` 和 minutes 命令必须使用 User Token。
- `vc bot meeting-join/meeting-leave` 默认 Bot 身份，而且只在显式 flag 时切换 User Token。
- `vc bot meeting-events` 支持 `--as bot|user|auto`（默认 auto），建议按来源显式选身份，须与
  `meeting_id` 来源一致：`--as user` 预检 `vc:meeting.meetingevent:read`；`--as bot`
  确认应用已开通 `vc:meeting.bot.join:write` 且机器人须在会中，不能用 User `auth check` 替代。`--as auto` 刷新/token 文件错误
  fail-closed，禁止静默切 Bot；`--dry-run` 只静态探测身份，不联网不写 token。

下载媒体时保留服务端文件名；无法解析扩展名时再按 Content-Type 推导。

搜索会议并下载妙记逐字稿时至少预检：

```bash
feishu-cli auth check --scope "vc:meeting.search:read vc:note:read minutes:minutes:readonly minutes:minutes.transcript:export"
```

遇到 Token、身份或 scope 报错（如 99991663/99991668/99991672/99991679）时，读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md` 确认应使用的身份与预检方式，排错表见 `../feishu-cli-platform/references/workflows/auth/workflow.md`。
