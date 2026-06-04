---
name: feishu-cli-vc
description: >-
  飞书视频会议与妙记操作。多维搜索历史会议、获取会议纪要/AI 产物/逐字稿、
  查询会议录制、下载妙记媒体文件、会议机器人入会/离会/会议事件。支持 meeting-ids /
  minute-tokens / calendar-event-ids 三路径入口。当用户请求"搜索会议"、"会议记录"、
  "会议纪要"、"逐字稿"、"妙记"、"meeting"、"vc search"、"vc recording"、"minutes"、
  "下载妙记"、"妙记视频"、"会议录制"、"会议机器人"、"机器人入会"、"vc bot"、
  "从日程找会议"时使用。
argument-hint: vc search | vc notes | vc recording | vc bot | minutes get | minutes download
user-invocable: true
allowed-tools: Bash(feishu-cli vc:*), Bash(feishu-cli minutes:*), Bash(feishu-cli auth:*), Read
---

# 飞书视频会议与妙记

搜索历史会议、获取纪要/AI 产物/逐字稿、查询会议录制、下载妙记媒体。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 前置条件

- **认证**：vc 读类（`vc search/notes/recording`、`vc bot meeting-events`）与 minutes 全部命令需要 **User Access Token**（推荐先 `auth check --scope "..."`，再执行 `feishu-cli auth login --scope "..."` 或 `--domain vc --domain minutes --recommend`）；**例外**：`vc bot meeting-join` / `vc bot meeting-leave` 默认走 **Bot/Tenant 身份**，仅靠 App ID + App Secret 即可，无需登录（详见第 6 节与「注意事项」的身份说明）
- **App 凭证**：应用 App ID + App Secret（环境变量 `FEISHU_APP_ID` + `FEISHU_APP_SECRET` 或 `~/.feishu-cli/config.yaml`）
- **预检**：`feishu-cli auth status` 查看登录状态；`feishu-cli auth check --scope "vc:meeting.search:read"` 预检 scope

## 命令速查

### 1. 搜索历史会议（多维过滤）

```bash
feishu-cli vc search [过滤条件]
```

底层走 `POST /open-apis/vc/v1/meetings/search`。**至少指定一个过滤条件**。

| 参数 | 类型 | 说明 |
|------|------|------|
| `--query` | string | 关键词（1-50 字符） |
| `--start` | string | 起始时间（YYYY-MM-DD 或 RFC3339） |
| `--end` | string | 结束时间（YYYY-MM-DD 或 RFC3339，纯日期自动对齐 23:59:59） |
| `--organizer-ids` | string | 主持人 open_id 列表，逗号分隔 |
| `--participant-ids` | string | 参会者 open_id 列表，逗号分隔 |
| `--room-ids` | string | 会议室 ID 列表，逗号分隔 |
| `--page-size` | int | 每页数量（1-30，默认 15） |
| `--page-token` | string | 分页标记 |
| `-o, --output` | string | `json` 格式化输出 |

### 2. 获取会议纪要（三路径）

```bash
feishu-cli vc notes (--meeting-ids | --minute-tokens | --calendar-event-ids) [选项]
```

三入口**互斥**，均支持逗号分隔批量（最多 50 条）。

| 参数 | 类型 | 说明 |
|------|------|------|
| `--meeting-ids` | CSV | 会议 ID 列表 |
| `--minute-tokens` | CSV | 妙记 token 列表 |
| `--calendar-event-ids` | CSV | 日历事件实例 ID 列表（自动反查 meeting_ids + meeting_notes） |
| `--with-artifacts` | bool | 额外获取 AI 产物（summary / todos / chapters） |
| `--download-transcript` | bool | 下载逐字稿到 `{output-dir}/artifact-{sanitized_title}-{token}/transcript.txt`（已存在时需加 `--overwrite`） |
| `--output-dir` | string | 逐字稿落盘目录（默认当前目录） |
| `--overwrite` | bool | 覆盖已存在的逐字稿文件 |
| `-o, --output` | string | `json` 格式化输出 |

JSON 输出结构：顶层为 `{items, summary}`；每个 `items[]` 为 `{id, ok, data, error}`。成功项的会议字段位于 `items[].data`，包括 `source / meeting_id / minute_token / title / minute_url / create_time / note_doc / verbatim_doc / shared_docs / artifacts / transcript_path`。

### 3. 查询会议录制 → minute_token

```bash
feishu-cli vc recording (--meeting-ids | --calendar-event-ids)
```

从会议录制 URL 中提取 `minute_token`，用于后续下载媒体或获取妙记。互斥入口，批量最多 50 条。输出字段为 `meeting_id / minute_token / recording_url / duration`，没有 `status` 字段。

| 参数 | 类型 | 说明 |
|------|------|------|
| `--meeting-ids` | CSV | 会议 ID 列表 |
| `--calendar-event-ids` | CSV | 日历事件实例 ID 列表 |
| `-o, --output` | string | `json` 格式化输出 |

### 4. 获取妙记基础信息（可合并 AI 产物）

```bash
feishu-cli minutes get <minute_token> [--with-artifacts] [-o json]
```

| 参数 | 说明 |
|------|------|
| `<minute_token>` | 位置参数，必填 |
| `--with-artifacts` | 额外调用 artifacts API 合并输出（summary / todos / chapters） |
| `-o, --output json` | JSON 格式输出 |

### 5. 下载妙记媒体文件（批量）

```bash
feishu-cli minutes download --minute-tokens <t1,t2,...> [--output <path>] [--overwrite] [--url-only]
```

先调 `GET /open-apis/minutes/v1/minutes/{token}/media` 拿预签名 URL，再走 HTTP 流式下载。内置 SSRF 防护（拒绝 localhost/回环/内网段）、重定向校验（最多 5 次、禁止 HTTPS→HTTP 降级）、文件名解析（Content-Disposition / RFC 5987 `filename*` / Content-Type 推导扩展名）、批量文件名冲突去重（加 `{token}-` 前缀）、5 req/s 速率限制（`time.Ticker`）。

| 参数 | 类型 | 说明 |
|------|------|------|
| `--minute-tokens` | CSV | **必填**，最多 50 条 |
| `--output` | string | 输出路径：单 token 为文件或目录；批量必须是目录；默认当前目录 |
| `--overwrite` | bool | 覆盖已存在文件 |
| `--url-only` | bool | 只打印下载 URL，不实际下载 |

### 6. 会议机器人入会 / 离会 / 会议事件

```bash
feishu-cli vc bot meeting-join   --meeting-number 123456789 [--password 1234] [--dry-run]
feishu-cli vc bot meeting-leave  --meeting-id 6911188411932033028 [--dry-run]
feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --start 2026-03-01 --end 2026-03-31
```

让会议机器人按会议号加入会议、离开会议，以及查询机器人侧的会议事件。

| 子命令 | 端点 | 身份 | 关键参数 |
|------|------|------|---------|
| `meeting-join` | `POST /open-apis/vc/v1/bots/join` | 默认 Bot/Tenant | `--meeting-number`（必填）、`--password`（可选）、`--dry-run`、`-o json` |
| `meeting-leave` | `POST /open-apis/vc/v1/bots/leave` | 默认 Bot/Tenant | `--meeting-id`（必填）、`--dry-run`、`-o json` |
| `meeting-events` | `GET /open-apis/vc/v1/bots/events` | 必须 User Token | `--meeting-id`（必填）、`--start`、`--end`、`--page-size`（20-100，默认 20）、`--page-token`、`--dry-run`、`-o json` |

> 三个子命令均支持 `--dry-run`（只打印将要发送的请求参数/请求体，不实际调用）与 `-o json`（输出原始响应）。
> `meeting-join` / `meeting-leave` 默认用 Bot/Tenant 身份，无需登录；只有显式传 `--user-access-token` flag 才切到 User 身份（这两个命令用 `resolveFlagUserToken`，**不读** `FEISHU_USER_ACCESS_TOKEN` 环境变量）。
> `meeting-events` 端点不接受 Tenant Token：已登录会自动用 User Token，未登录会被 `99991663` 拒绝 → 需先 `feishu-cli auth login`。
> `meeting-events` 的 `--page-size` 取值范围是 **20-100**（与 `vc search` 的 1-30 不同）；传 0 或不传走默认 20，传 1-19 会被拒。

## 使用示例

```bash
# 关键词 + 时间范围搜索
feishu-cli vc search --query "周会" --start 2026-03-20 --end 2026-04-11

# 按主持人过滤 + JSON 输出
feishu-cli vc search --organizer-ids ou_xxx,ou_yyy -o json

# 通过会议 ID 批量查纪要
feishu-cli vc notes --meeting-ids 6900001,6900002

# 通过妙记 token 查 + 获取 AI 产物 + 下载逐字稿
feishu-cli vc notes --minute-tokens obcnxxxx \
  --with-artifacts --download-transcript --output-dir ./notes

# 从日历事件反查并下载全部逐字稿
feishu-cli vc notes --calendar-event-ids <event_id> --download-transcript --output-dir ./notes

# 从会议 ID 反查 minute_token
feishu-cli vc recording --meeting-ids 6900001 -o json

# 单条妙记下载到当前目录（自动解析文件名）
feishu-cli minutes download --minute-tokens obcnxxxx

# 批量下载到指定目录
feishu-cli minutes download --minute-tokens t1,t2,t3 --output ./media --overwrite

# 只取下载链接不下载
feishu-cli minutes download --minute-tokens obcnxxxx --url-only

# 获取妙记信息并展示 AI 摘要
feishu-cli minutes get obcnxxxx --with-artifacts

# 机器人按会议号入会（默认 Bot/Tenant 身份，无需登录）
feishu-cli vc bot meeting-join --meeting-number 123456789 --password 1234

# 预览离会请求体不实际调用
feishu-cli vc bot meeting-leave --meeting-id 6911188411932033028 --dry-run

# 查询机器人会议事件（必须 User Token），分页 + JSON 输出
feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 \
  --start 2026-03-01 --end 2026-03-31 --page-size 50 -o json
```

## 典型工作流

### 工作流 A：会议搜索 → 录制 → 妙记媒体

```bash
# 1. 搜索目标会议
feishu-cli vc search --query "架构评审" --start 2026-03-01 -o json
# → 记录 meeting_id

# 2. 查会议录制，拿 minute_token
feishu-cli vc recording --meeting-ids <meeting_id> -o json
# → 记录 minute_token

# 3. 下载媒体文件
feishu-cli minutes download --minute-tokens <minute_token> --output ./media
```

### 工作流 B：日历事件直达妙记下载

```bash
# 1. 从日历事件一次性拿到纪要、AI 产物、逐字稿
feishu-cli vc notes --calendar-event-ids <event_id> \
  --with-artifacts --download-transcript --output-dir ./notes -o json

# 2. 若要下载音视频，配合 recording 命令
feishu-cli vc recording --calendar-event-ids <event_id> -o json
# → 取得 minute_token
feishu-cli minutes download --minute-tokens <minute_token> --output ./media
```

## 权限要求

> 以下为最小所需精确 scope；项目顶层 CLAUDE.md 用 `minutes:minutes*:*` 通配等价覆盖（`minutes:minutes:readonly` + `minutes:minutes.artifacts:read` + `minutes:minutes.media:export` + `minutes:minutes.transcript:export`）。

| 命令 / 功能 | 必需 scope |
|------|---------|
| `vc search` | `vc:meeting.search:read` |
| `vc notes`（meeting-ids 路径） | `vc:meeting.meetingevent:read`、`vc:note:read` |
| `vc notes`（minute-tokens 路径） | `minutes:minutes:readonly`、`vc:note:read` |
| `vc notes --with-artifacts` | + `minutes:minutes.artifacts:read` |
| `vc notes --download-transcript` | + `minutes:minutes.transcript:export` |
| `vc notes`（calendar-event-ids 路径） | + `calendar:calendar:read`、`calendar:calendar.event:read` |
| `vc recording` | `vc:record:readonly`（calendar 路径同上追加日历权限） |
| `vc bot meeting-join` | `vc:meeting.bot.join:write` |
| `vc bot meeting-leave` | `vc:meeting.bot.leave:write` |
| `vc bot meeting-events` | vc 读权限（必须 User Token；未登录会被 `99991663` 拒绝） |
| `minutes get` | `minutes:minutes:readonly`（`--with-artifacts` 额外需 `minutes:minutes.artifacts:read`） |
| `minutes download` | `minutes:minutes.media:export` |

权限在飞书开放平台的应用权限管理页面开通；开通后执行 `feishu-cli auth login --scope "所需 scope..."` 或 `feishu-cli auth login --domain vc --domain minutes --recommend` 重新授权即可。

## 注意事项

- **Token 身份分三档**：
  - **必须 User Token**：`vc search/notes/recording`、`vc bot meeting-events`、`minutes get/download`。未登录会中文报错并引导 `feishu-cli auth login`；`meeting-events` 端点不接受 Tenant Token，未登录直接被 `99991663` 拒绝。
  - **默认 Bot/Tenant**：`vc bot meeting-join` / `vc bot meeting-leave`，仅需 App ID + App Secret，无需登录；只有显式传 `--user-access-token` flag 才切到 User 身份（用 `resolveFlagUserToken`，**不读** `FEISHU_USER_ACCESS_TOKEN` 环境变量）。
  - 覆盖登录态：除 `meeting-join`/`meeting-leave`（仅认 `--user-access-token` flag）外，其余命令也可用 `FEISHU_USER_ACCESS_TOKEN` 环境变量。
- **时间格式**：`vc search --start/--end` 接受 `YYYY-MM-DD` / `YYYY-MM-DD HH:MM:SS` / RFC3339，均按本地时区解析；纯日期的 `--end` 自动对齐到 23:59:59。
- **批量上限**：所有 CSV 类入参统一 50 条上限，超出直接报错。
- **minute_token 格式**：字母数字组合，长度≥5；命令会前置校验。
- **calendar-event-id**：指"日历事件实例 ID"，不是日程 event_id 本身；可从 `feishu-cli calendar agenda` 或日程视图 API 获取。
- **文件名解析**：`minutes download` 按 Content-Disposition > Content-Type 扩展 > `{token}.media` 的优先级决定文件名；批量模式冲突时自动加 `{token}-` 前缀。
- **SSRF 防护**：下载 URL 会被校验，拒绝指向内网段 / localhost / 非 http(s) scheme；重定向最多 5 次且禁止 HTTPS → HTTP 降级。
- **数据时效**：会议结束后一段时间才能查到纪要/妙记；实时会议无法获取。
- **逐字稿去重**：同一 `vc notes` 调用中同一 `minute_token` 的逐字稿只下载一次。
