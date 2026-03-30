---
name: feishu-cli-calendar
description: >-
  飞书日历：查看日历、创建和更新日程、管理参与人、查询忙闲状态和搜索日程。
  适用于预约会议、查空闲时间、查看今天/本周日程等场景。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书日历技能

这个技能对齐官方 lark-calendar 的入口。

## 常用命令

```bash
feishu-cli calendar list
feishu-cli calendar primary
feishu-cli calendar get <calendar_id>
feishu-cli calendar create-event --calendar-id <calendar_id> --summary "团队周会" --start "2026-03-30T10:00:00+08:00" --end "2026-03-30T11:00:00+08:00"
feishu-cli calendar get-event <calendar_id> <event_id>
feishu-cli calendar list-events <calendar_id>
feishu-cli calendar update-event <calendar_id> <event_id> --summary "新标题"
feishu-cli calendar delete-event <calendar_id> <event_id>
feishu-cli calendar event-search --calendar-id <calendar_id> --query "周会"
feishu-cli calendar event-reply <calendar_id> <event_id> --status accept
feishu-cli calendar attendee list <calendar_id> <event_id>
feishu-cli calendar freebusy --start "2026-03-30T00:00:00+08:00" --end "2026-03-31T00:00:00+08:00"
feishu-cli calendar agenda <calendar_id>
```

## 使用建议

- 用户要“帮我约个会”时，先查 busy 再创建
- 用户要“今天有什么安排”时，先看 `agenda`
- 参会人和时间有歧义时，先澄清再创建
