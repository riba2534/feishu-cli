# 飞书视频会议与妙记

搜索历史会议、获取纪要/AI 产物/逐字稿、查询会议录制、下载妙记媒体。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 目录

- [前置条件](#前置条件)
- [命令速查](#命令速查)
- [使用示例](#使用示例)
- [典型工作流](#典型工作流)
- [权限要求](#权限要求)
- [注意事项](#注意事项)

## 前置条件

- **认证**：`vc search/notes/recording/detail`、`vc note detail`、`vc meeting list-active` 与 minutes 读命令支持 `--as user|bot|auto`，**默认 user**（需 User Access Token）；`vc note transcript` 只支持 User；`vc bot meeting-join/meeting-leave` 只支持 Bot（仅靠 App ID + App Secret，传 `--user-access-token` 直接报错）。`vc bot meeting-events` 用显式 `--as bot|user|auto`（默认 auto），身份必须与 `meeting_id` 来源一致。本地 User 路径先 `auth check --scope "..."`，Bot 路径确认应用权限，不能以 User 登录作为前提；通用规则见 `feishu-cli-platform` 的 auth 身份参考，下文补充会议特有约束
- **App 凭证**：应用 App ID + App Secret（环境变量 `FEISHU_APP_ID` + `FEISHU_APP_SECRET` 或 `~/.feishu-cli/config.yaml`）
- **User 搜索预检**：使用本地登录身份执行 `vc search` 前，可用 `feishu-cli auth status` 查看状态、`feishu-cli auth check --scope "vc:meeting.search:read"` 检查 scope。Bot 入会或查询事件不要求这个 User scope。

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
| `--as` | string | 身份：`user`（默认）/ `bot` / `auto` |
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
| `--as` | string | 身份：`user`（默认）/ `bot` / `auto` |
| `-o, --output` | string | `json` 格式化输出 |

JSON 输出结构：顶层为 `{items, summary}`；每个 `items[]` 为 `{id, ok, data, error}`。成功项的会议字段位于 `items[].data`，包括 `source / meeting_id / minute_token / note_id / note_display_type / title / minute_url / create_time / note_doc / verbatim_doc / shared_docs / artifacts / transcript_path / hint`。

- `meeting-ids` 路径始终经录制接口补 `minute_token`（best-effort）。
- `note_display_type`：`normal`（普通纪要，逐字稿在 `verbatim_doc` 文档里，用 `doc export` 读取）/ `unified`（统一纪要，可用 `vc note transcript`）/ `unknown`。
- 会议没有纪要、纪要无权限（121005）、录制查询失败等不中断整条结果，写入 `hint`。

### 3. 查询会议录制 → minute_token

```bash
feishu-cli vc recording (--meeting-ids | --calendar-event-ids)
```

从会议录制 URL 中提取 `minute_token`，用于后续下载媒体或获取妙记。互斥入口，批量最多 50 条。输出字段为 `meeting_id / minute_token / recording_url / duration`，没有 `status` 字段。

| 参数 | 类型 | 说明 |
|------|------|------|
| `--meeting-ids` | CSV | 会议 ID 列表 |
| `--calendar-event-ids` | CSV | 日历事件实例 ID 列表 |
| `--as` | string | 身份：`user`（默认）/ `bot` / `auto` |
| `-o, --output` | string | `json` 格式化输出 |

### 4. 获取妙记基础信息（可选择获取 AI 产物、可等待就绪）

```bash
feishu-cli minutes get <minute_token> [--summary] [--todo] [--chapter] [--keyword] [--transcript] [--wait-ready] [-o json]
```

| 参数 | 说明 |
|------|------|
| `<minute_token>` | 位置参数，必填 |
| `--summary` / `--todo` / `--chapter` / `--keyword` | 按需获取 AI 摘要 / 待办 / 章节 / 关键词，未选择时不调 artifacts 接口 |
| `--transcript` | 获取逐字稿并**写入文件**，输出中只给 `transcript_file` 路径（不再内联） |
| `--with-artifacts` | 兼容旧 flag，等价于以上 5 项全选（逐字稿同样写文件） |
| `--output-dir` | 逐字稿目录：默认 `./minutes/<token>/transcript.txt`；指定后为 `<dir>/artifact-<标题>-<token>/transcript.txt` |
| `--overwrite` | 覆盖已存在的逐字稿文件（默认保留原文件并在 stderr 提示） |
| `--as` | 身份：`user`（默认）/ `bot` / `auto` |
| `--wait-ready` | 妙记或 AI 产物仍在生成（转写未完成，业务码 `2091003`）时轮询等待，直到就绪或超时；适合刚结束的会议 |
| `--wait-timeout` | `--wait-ready` 最长等待秒数（默认 300） |
| `--wait-interval` | `--wait-ready` 轮询间隔秒数（默认 10） |
| `-o, --output json` | JSON 格式输出 |

> 妙记已就绪时 `--wait-ready` 立即返回，不产生额外等待；只有遇到 `2091003`（仍在生成）才会按 `--wait-interval` 轮询。

JSON 输出：`{minute, artifacts}`，`artifacts` 只含选中的键，沿用服务端字段名：`summary / minute_todos / minute_chapters / keywords / transcript_file`。
无权限（`2091005`，退出码 3）时会提示 `minutes apply-permission --minute-token <token> --perm view`；申请会通知妙记所有者，**先征得用户同意**再执行。

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
| `--as` | string | 身份：`user`（默认）/ `bot` / `auto` |

### 6. 会议机器人入会 / 离会 / 会议事件

```bash
feishu-cli vc bot meeting-join   --meeting-number 123456789 [--password 1234] [--call-id <id>] [--action join|start] [--dry-run]
feishu-cli vc bot meeting-leave  --meeting-id 6911188411932033028 [--dry-run]
feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --start 2026-03-01 --end 2026-03-31
```

让会议机器人按会议号加入会议、离开会议，以及查询机器人侧的会议事件。

| 子命令 | 端点 | 身份 | 关键参数 |
|------|------|------|---------|
| `meeting-join` | `POST /open-apis/vc/v1/bots/join` | **仅 Bot** | `--meeting-number`（必填，9 位数字）、`--password`、`--call-id`（邀请事件透传的关联 ID）、`--action join\|start`（start = 发起日程会议，请求体 `action=2`）、`--dry-run`、`-o json` |
| `meeting-leave` | `POST /open-apis/vc/v1/bots/leave` | **仅 Bot** | `--meeting-id`（必填）、`--dry-run`、`-o json` |
| `meeting-events` | `GET /open-apis/vc/v1/bots/events` | `--as bot\|user\|auto`（默认 auto） | `--meeting-id`（必填，长数字 meeting_id）、`--as`、`--start`、`--end`、`--page-size`（20-100，默认 20）、`--page-token`、`--page-all`、`--dry-run`、`-o json` |

> 三个子命令均支持 `--dry-run`（只打印将要发送的请求参数/请求体，不实际调用，预览含 `"as"`）与 `-o json`（输出原始响应）。
> 身份细节（`meeting-join/leave` 仅 Bot、传 `--user-access-token` 报错；`meeting-events` 建议按来源显式选择 `--as`；实调 auto fail-closed，dry-run 静态探测不联网）见「注意事项」的「Token 身份分三档」。
> `meeting-events` 的 `--page-size` 取值范围是 **20-100**（与 `vc search` 的 1-30 不同）；传 0 或不传走默认 20，传 1-19 会被拒。
> `meeting-events` 的事件列表字段是 `events`（JSON 输出原样透传 `events / has_more / page_token`）；`--page-all` 每页 100、最多 200 页、遇重复游标即停止，合并后保留末页 `has_more / page_token`。
> `meeting-events --meeting-id` 传 9 位会议号会被拒（exit 2）：先用 `vc meeting list-active`（进行中）或 `vc detail <会议号>`（已结束）拿 meeting_id。
> 读事件的常见服务端错误：`120002`（主持人未开启 AI 纪要或未允许 agent 入会）、`120003`（当前身份不在该会议中）。

### 6.1 查询进行中的会议（vc meeting list-active）

```bash
feishu-cli vc meeting list-active [-o json]
feishu-cli vc meeting list-active --as bot --user-id ou_xxx [-o json]
```

底层 `GET /open-apis/vc/v1/bots/user_active_meeting`，返回 `meetings[].meeting_id / meeting_no / meeting_title`，是 `vc bot meeting-events` 的前置。

| 参数 | 类型 | 说明 |
|------|------|------|
| `--as` | string | `user`（默认，查当前登录用户）/ `bot`（必须带 `--user-id`）/ `auto` |
| `--user-id` | string | 目标用户 open_id（`ou_` 开头，仅 Bot 身份使用） |
| `--dry-run` | bool | 只打印请求，不调用 |
| `-o, --output` | string | `json` 原样输出 |

同时在多个会议中时，先让用户确认要查询的 meeting_id，再调 `meeting-events`。

### 7. 聚合会议详情 → note_id + minute_token（vc detail）

```bash
feishu-cli vc detail <meeting_id 或会议号> [--start ...] [--end ...] [-o json]
```

一条命令串联 `GET /open-apis/vc/v1/meetings/{meeting_id}`（拿基础信息 + `note_id`）与 `GET .../recording`（提取 `minute_token`），把后续查纪要/妙记所需的所有 ID 一次拿齐，省去逐个调用。

| 参数 | 类型 | 说明 |
|------|------|------|
| `<meeting_id 或会议号>` | 位置参数 | 会议 ID（长数字串）或 **9 位会议号**；传会议号时先经 `list_by_no` 反查关联会议（周期性会议可能对应多场实例，分别输出） |
| `--start` | string | 会议号反查时间窗口起点（YYYY-MM-DD 或 RFC3339，默认近 90 天） |
| `--end` | string | 会议号反查时间窗口终点（默认当前时间） |
| `--as` | string | 身份：`user`（默认）/ `bot` / `auto` |
| `-o, --output` | string | `json` 格式化输出 |

输出字段：`meeting_id / meeting_no / topic / start_time / end_time / status / note_id / minute_token / hint`。`status` 为 `ongoing`（进行中，纪要与妙记尚未生成，相关字段留空并在 `hint` 说明）或 `ended`（已结束）。`note_id` 仅在会议关联了智能纪要时存在；未命中的字段会在 `hint` 里标注，不视为错误。

### 8. 搜索妙记（关键词/所有者/参与者/时间）

```bash
feishu-cli minutes search [过滤条件] [-o json]
```

底层走 `POST /open-apis/minutes/v1/minutes/search`，请求体固定 `sorter=create_time_desc`（按创建时间倒序），保证 `--page-token` 翻页不漏条、不重复。**至少指定一个过滤条件**。

| 参数 | 类型 | 说明 |
|------|------|------|
| `--query` | string | 关键词（1-50 字符；`--keyword` 为别名） |
| `--owner-ids` | string | 所有者 open_id 列表，逗号分隔（任一匹配；`me` = 当前登录用户） |
| `--participant-ids` | string | 参与者 open_id 列表，逗号分隔（任一匹配；`me` = 当前登录用户） |
| `--start` | string | 创建时间起点（YYYY-MM-DD 或 RFC3339） |
| `--end` | string | 创建时间终点（纯日期自动对齐 23:59:59） |
| `--as` | string | 身份：`user`（默认）/ `bot` / `auto` |
| `--page-size` | int | 每页数量（1-30，默认 15） |
| `--page-token` | string | 分页标记 |
| `-o, --output` | string | `json` 格式化输出（透传原始 `items/has_more/page_token`） |

文本模式会剥离 `display_info` 的高亮标签并取首行为标题，附 `token / 信息 / 链接`；`has_more` 时提示下一页 `--page-token`。
旧参数名 `--owner-id` / `--start-time` / `--end-time` 仍可用（stderr 提示废弃）；`me` 需要已登录 User 才能解析 open_id。

### 9. 申请妙记权限（view/edit）

```bash
feishu-cli minutes apply-permission --minute-token <token> --perm view|edit [-o json]
```

底层走 `POST /open-apis/minutes/v1/minutes/{minute_token}/permissions/apply`。当对某妙记无访问权限（如 `minutes get` 返回 `2091005` 无权限）时，用本命令发起申请。

| 参数 | 类型 | 说明 |
|------|------|------|
| `--minute-token` | string | **必填**，妙记 Token |
| `--perm` | string | **必填**，`view`（查看）或 `edit`（编辑） |
| `--as` | string | 身份：`user`（默认）/ `bot` / `auto` |
| `-o, --output` | string | `json` 格式化输出 |

> 申请会通知妙记所有者，执行前先征得用户同意。

### 10. 智能纪要（vc note，按 note_id 直接操作）

```bash
feishu-cli vc note detail <note_id> [--as user|bot|auto]    # 纪要详情（展示类型、关联文档 token）
feishu-cli vc note transcript <note_id> \
  [--format markdown|plain_text] [--locale zh_cn] [--output transcript.md]   # 统一逐字稿导出（自动翻页拉全量）
```

`detail` 无 `-o/--output` flag（输出格式固定为 JSON）；`--output`（保存路径）、`--format`、`--locale` 仅 `transcript` 支持。
`note_id` 来自 `vc detail` / `vc notes` 结果（仅关联智能纪要的会议才有）。与 `vc notes` 的区别：
`vc notes` 按会议/妙记批量查产物，`vc note` 按 note_id 直接操作单篇纪要。

**纪要类型决定逐字稿读法**（`vc note detail` 的 `note.note_display_type`）：

| note_display_type | 含义 | 逐字稿读法 |
|---|---|---|
| `1`（normal） | 普通纪要（目前绝大多数会议纪要） | `artifacts[]` 中 `artifact_type=2` 的 `doc_token` 是逐字稿文档：`feishu-cli doc export <verbatim_doc_token> -o transcript.md` |
| `2`（unified） | 统一纪要 | `feishu-cli vc note transcript <note_id>` |

`vc note transcript` 先查纪要详情：普通纪要（或服务端返回 `121002 not support`）直接报错（exit 2）并给出 `doc export <verbatim_doc_token>` 命令，不会输出半截内容。
统一纪要按 `format`（`markdown` 默认 / `plain_text`；旧值 `text` 视为 `plain_text`）、`page_size=200`、`locale`（默认飞书 `zh_cn`、Lark `en_us`）拉取，`next_cursor_id` 自动翻页（重复游标或超过 500 页即报错）。
权限：`vc:note:read`；`transcript` 只支持 User，`detail` 默认 User、可 `--as bot|auto`；`121005` 表示无该纪要阅读权限。

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
feishu-cli minutes get obcnxxxx --summary

# 刚结束的会议，等待妙记转写完成再取（最长 10 分钟）
feishu-cli minutes get obcnxxxx --wait-ready --wait-timeout 600

# 用会议 ID 一次拿齐 note_id + minute_token
feishu-cli vc detail 6911188411932033028 -o json

# 用 9 位会议号聚合查询（自动反查关联会议）
feishu-cli vc detail 543343946 --start 2026-06-01 --end 2026-07-22

# 搜索我拥有的、含关键词的妙记
feishu-cli minutes search --query "预算复盘" --owner-ids me -o json

# 我参与过的妙记（按创建时间倒序）
feishu-cli minutes search --participant-ids me --start 2026-03-01

# 只取 AI 摘要与待办；逐字稿写文件
feishu-cli minutes get obcnxxxx --summary --todo -o json
feishu-cli minutes get obcnxxxx --transcript --output-dir ./notes

# 对无权限的妙记申请查看权限
feishu-cli minutes apply-permission --minute-token obcnxxxx --perm view

# 机器人按会议号入会（仅 Bot 身份，无需登录）
feishu-cli vc bot meeting-join --meeting-number 123456789 --password 1234

# 查看当前进行中的会议，拿 meeting_id
feishu-cli vc meeting list-active

# 预览离会请求体不实际调用
feishu-cli vc bot meeting-leave --meeting-id 6911188411932033028 --dry-run

# 查询会议事件：身份必须与 meeting_id 来源一致（用户发现用 --as user，机器人入会用 --as bot）
feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 \
  --as user --start 2026-03-01 --end 2026-03-31 --page-size 50 -o json
feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --as bot --dry-run
feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --as user --page-all -o json
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

### 工作流 C：读取会议逐字稿（按纪要类型分流）

```bash
# 1. 拿 note_id、minute_token 与纪要类型
feishu-cli vc notes --meeting-ids <meeting_id> -o json
# → items[].data.note_display_type / verbatim_doc / minute_token

# 2a. normal 纪要：导出 verbatim 逐字稿文档
feishu-cli doc export <verbatim_doc> -o transcript.md
# 2b. unified 纪要：导出统一逐字稿
feishu-cli vc note transcript <note_id> --output transcript.md
# 2c. 只有妙记：妙记逐字稿写文件
feishu-cli minutes get <minute_token> --transcript
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
| `vc bot meeting-leave` | `vc:meeting.bot.join:write`（与入会同一 scope） |
| `vc bot meeting-events` | User：`vc:meeting.meetingevent:read`；Bot：`vc:meeting.bot.join:write`。支持 `--as bot\|user\|auto`；已配置 User 不可用时不能静默回落 |
| `vc meeting list-active` | User：`vc:meeting.meetingevent:read`；Bot：`vc:meeting.bot.join:write` |
| `vc note detail` / `vc note transcript` | `vc:note:read` |
| `vc detail`（meeting_id 路径） | `vc:meeting.meetingevent:read`、`vc:record:readonly` |
| `vc detail`（会议号路径） | + `vc:meeting:readonly` 或 `vc:meeting.meetingid:read`（`list_by_no` 反查所需） |
| `minutes get` | `minutes:minutes:readonly`（选择任一 AI 产物或 `--with-artifacts` 额外需 `minutes:minutes.artifacts:read`） |
| `minutes search` | `minutes:minutes.search:read` |
| `minutes apply-permission` | `minutes:permission:apply` |
| `minutes download` | `minutes:minutes.media:export` |

读命令以 `--as bot` 调用时，上表 scope 需在应用侧（Tenant）开通；Bot 权限在飞书开放平台的应用权限管理页面开通；User 路径还需执行 `feishu-cli auth login --scope "所需 scope..."` 或 `feishu-cli auth login --domain vc --domain minutes --recommend` 重新授权即可。

## 注意事项

- **Token 身份分三档**：
  - **默认 User、可 `--as` 切换**：`vc search/notes/recording/detail`、`vc note detail`、`vc meeting list-active`、`minutes get/search/apply-permission/download`。默认 `--as user`，未登录会中文报错并引导 `feishu-cli auth login`；`--as bot` 走 App Token（需应用开通 scope），`--as auto` User 优先、未配置回退 Bot、已配置但刷新失败 fail-closed。`vc note transcript` 只支持 User。
  - **仅 Bot**：`vc bot meeting-join` / `vc bot meeting-leave`，仅需 App ID + App Secret，无需登录；传 `--user-access-token` 直接报用法错误（exit 2），不会以用户身份入会/离会。
  - **显式 `--as`**：`vc bot meeting-events` 支持 `--as bot|user|auto`（默认 auto）。`--as user` 缺 Token 失败；`--as bot` 即使已登录也走 Bot；`--as auto` 已登录用 User、未登录用 Bot，刷新或 token 文件错误 fail-closed（禁止静默切 Bot）。dry-run 只做静态身份探测（不刷新、不联网、不写 token），预览 JSON 含 `"as"`。身份必须与 `meeting_id` 来源一致。
- **时间格式**：`vc search --start/--end` 接受 `YYYY-MM-DD` / `YYYY-MM-DD HH:MM:SS` / RFC3339，均按本地时区解析；纯日期的 `--end` 自动对齐到 23:59:59。
- **批量上限**：所有 CSV 类入参统一 50 条上限，超出直接报错。
- **minute_token 格式**：字母数字组合，长度≥5；命令会前置校验。
- **calendar-event-id**：指"日历事件实例 ID"，不是日程 event_id 本身；可从 `feishu-cli calendar agenda` 或日程视图 API 获取。
- **文件名解析**：`minutes download` 按 Content-Disposition > Content-Type 扩展 > `{token}.media` 的优先级决定文件名；批量模式冲突时自动加 `{token}-` 前缀。
- **SSRF 防护**：下载 URL 会被校验，拒绝指向内网段 / localhost / 非 http(s) scheme；重定向最多 5 次且禁止 HTTPS → HTTP 降级。
- **数据时效**：会议结束后一段时间才能查到纪要/妙记；进行中的会议只能用 `vc meeting list-active` + `vc bot meeting-events` 读会中事件。
- **错误码**：业务错误随 HTTP 4xx 下发时按业务码处理（例如 `2091005` 妙记无权限、`121005` 纪要无权限、`121002` 非统一纪要）；错误信息附 `log_id` 便于排查。
- **逐字稿去重**：同一 `vc notes` 调用中同一 `minute_token` 的逐字稿只下载一次。
