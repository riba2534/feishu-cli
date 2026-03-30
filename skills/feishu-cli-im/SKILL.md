---
name: feishu-cli-im
description: >-
  飞书即时通讯：发送消息、回复消息、转发、合并转发、批量获取消息、
  下载消息资源和获取话题回复列表。群聊浏览、历史记录、搜索和成员管理请改用
  feishu-cli-chat。发送类操作优先用 App Token，读取类场景按命令要求切换身份。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书即时通讯技能

这个技能对齐官方 lark-im 的“消息发送/互动”入口，具体命令使用 feishu-cli 已有实现。

## 适用范围

- 给人或群发送消息
- 回复、转发、合并转发消息
- 批量获取消息详情
- 下载图片 / 文件资源
- 获取 thread 消息列表

## 常用命令

```bash
feishu-cli msg send --receive-id-type email --receive-id user@example.com --text "Hello"
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx --msg-type post --content-file msg.json
feishu-cli msg reply <message_id> --text "收到"
feishu-cli msg forward <message_id> --receive-id-type email --receive-id user@example.com
feishu-cli msg merge-forward <message_id>
feishu-cli msg mget --message-ids om_xxx,om_yyy
feishu-cli msg resource-download <message_id> <file_key> -o photo.png
feishu-cli msg thread-messages <thread_id> --page-size 20
```

## 边界

- 群聊搜索、消息历史、Pin、Reaction、群成员管理请看 [feishu-cli-chat](../feishu-cli-chat/SKILL.md)
- 如果用户在问“怎么发消息”，优先走这个技能

