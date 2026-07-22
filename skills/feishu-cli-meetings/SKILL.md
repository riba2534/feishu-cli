---
name: feishu-cli-meetings
description: >-
  飞书视频会议与妙记专用入口，覆盖会议搜索、纪要与 AI 产物、逐字稿、录制、妙记媒体下载、
  会议机器人入会/离会和会议事件。用户要求按日程或时间查找历史会议、获取会议纪要/AI 摘要、
  minute token、下载录制/视频/逐字稿、操作 meeting bot 或查询妙记时必须使用本 Skill。创建日程和找会议时间
  使用 feishu-cli-work。
argument-hint: <vc|minutes> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Read, Write
---

# 飞书会议与妙记

读取 `references/workflows/vc/workflow.md` 后执行。
将该工作流中的 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按 `workflow.md`
所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 身份边界

- `vc search/notes/recording/detail` 和 minutes 命令必须使用 User Token。
- `vc bot meeting-join/meeting-leave` 默认 Bot 身份，而且只在显式 flag 时切换 User Token。
- `vc bot meeting-events` 端点拒收 Tenant Token，应使用 User Token，并预检
  `vc:meeting.meetingevent:read`。

下载媒体时保留服务端文件名；无法解析扩展名时再按 Content-Type 推导。

搜索会议并下载妙记逐字稿时至少预检：

```bash
feishu-cli auth check --scope "vc:meeting.search:read vc:note:read minutes:minutes:readonly minutes:minutes.transcript:export"
```
