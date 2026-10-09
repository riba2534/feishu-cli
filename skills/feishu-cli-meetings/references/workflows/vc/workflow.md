# 飞书视频会议、纪要与妙记

覆盖 `vc` 与 `minutes` 全部命令：搜索与定位历史会议、智能纪要与逐字稿、妙记（搜索、详情与 AI 产物、
媒体下载、权限申请）、进行中的会议与会中事件、会议机器人入会/离会。完整参数以 `feishu-cli <命令> --help` 为准，
本文只记录非显然的约束、默认值和组合用法。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 目录

- [适用范围与边界](#适用范围与边界)
- [标识与对象关系](#标识与对象关系)
- [身份与前置检查](#身份与前置检查)
- [决策规则](#决策规则)
- [命令：搜索与定位会议](#命令搜索与定位会议)
- [命令：纪要与逐字稿](#命令纪要与逐字稿)
- [命令：妙记](#命令妙记)
- [命令：进行中的会议与会中事件](#命令进行中的会议与会中事件)
- [命令：会议机器人入会/离会](#命令会议机器人入会离会)
- [典型工作流](#典型工作流)
- [权限要求](#权限要求)
- [坑点与错误处理](#坑点与错误处理)

## 适用范围与边界

- 本工作流：`vc search/detail/recording/notes`、`vc note detail/transcript`、`vc meeting list-active`、
  `vc bot meeting-join/meeting-leave/meeting-events`、`minutes search/get/download/apply-permission`。
- 创建或修改日程、找共同空闲时间、预订会议室：`feishu-cli-work` 的 calendar 工作流
  （`feishu-cli-work/references/workflows/calendar/workflow.md`）。
- 普通纪要的逐字稿是一篇云文档，用 `doc export` 导出（`feishu-cli-docs/references/workflows/export/workflow.md`）。
- 订阅"会议开始/结束、纪要已生成、录制/逐字稿已生成"等实时事件（`vc.meeting.*`、`vc.note.generated_v1`、
  `vc.recording.*`）：`feishu-cli-messaging/references/workflows/event/workflow.md`。
- CLI 不提供妙记编辑类命令，也不提供会中控制（邀请、结束会议、会中发消息、截图、倒计时）。

## 标识与对象关系

| 标识 | 形态 | 从哪里拿 | 用在哪里 |
|---|---|---|---|
| `meeting_id` | 长数字串（如 `6911188411932033028`） | `vc search` 的 `items[].id`、`vc detail`、`vc meeting list-active` | `vc detail/notes/recording`、`meeting-events`、`meeting-leave` |
| 会议号 | 恰好 9 位纯数字 | 用户口述、会议链接 | 只用于 `vc detail <会议号>`、`vc search --query <会议号>`、`meeting-join --meeting-number` |
| `note_id` | 长数字串 | `vc detail`、`vc notes` | `vc note detail/transcript` |
| `minute_token` | 字母数字（≥5 位），妙记 URL `/minutes/<token>` 的末段 | `vc detail`、`vc notes`、`vc recording`、`minutes search` | `minutes get/download/apply-permission`、`vc notes --minute-tokens` |
| 日历实例 ID | `<uuid>_0` 或 `<uuid>_<时间戳>` | `calendar agenda -o json` 的 `events[].event_id` | `vc notes/recording --calendar-event-ids` |
| 文档 token | `note_doc` / `verbatim_doc` / `shared_docs` | `vc notes`、`vc note detail` | `doc export` |

- 标识按字符串原样传递，不能互换；9 位会议号不能当 `meeting_id` 用。
- 智能纪要（Note，AI 总结链路）与妙记（Minutes，录制链路）相互独立：一场会议可能两者都有、只有一个或都没有。
  `vc detail` / `vc notes` 在 `hint` 中说明缺了哪个，这不是错误。

## 身份与前置检查

| 命令 | 身份 |
|---|---|
| `vc search/detail/recording/notes`、`vc note detail`、`vc meeting list-active`、`minutes search/get/download/apply-permission` | `--as user\|bot\|auto`，**默认 user** |
| `vc note transcript` | 只支持 User（没有 `--as`） |
| `vc bot meeting-join` / `meeting-leave` | 只支持 Bot：只用 App 凭证，不读取 `FEISHU_USER_ACCESS_TOKEN`；传 `--user-access-token` 报用法错误（exit 2） |
| `vc bot meeting-events` | `--as bot\|user\|auto`，默认 auto；必须与 `meeting_id` 的来源身份一致 |

- `--as auto`：User 优先、未配置时回退 Bot；已配置 User 但解析或刷新失败时直接报错，不静默切 Bot。
  `meeting-events --dry-run` 只做静态身份探测（不联网、不刷新、不写 token），预览 JSON 含 `"as"`。
- `--as bot` 需要应用侧开通对应 scope，缺失时报 99991672（重新 `auth login` 无法解决）。即使 scope 齐全，
  Bot 对用户的会议和妙记通常也没有资源权限（实测 `minutes get --as bot` 读用户妙记返回 2091005），
  读取用户自己的会议、纪要和妙记时用默认的 user。
- 身份延续：取得 `meeting_id` / `note_id` / `minute_token` 时用的身份，后续命令沿用。目标命令不支持该身份
  （例如 Bot 查到的纪要要导出统一逐字稿）时，说明限制并停下，经用户同意再换身份。
- User 路径业务命令前先预检，例如：

```bash
feishu-cli auth check --scope "vc:meeting.search:read vc:note:read minutes:minutes:readonly minutes:minutes.artifacts:read"
```

## 决策规则

1. **定位会议**
   - 已有 `meeting_id`：直接 `vc detail <meeting_id>`，一次拿到状态、`note_id`、`minute_token`。
   - 只有 9 位会议号：`vc detail <会议号>`（额外需要 `vc:meeting:readonly` 或 `vc:meeting.meetingid:read`）；
     缺这个 scope 时改用 `vc search --query <会议号>`，取 `items[].id`。
   - 只有主题、时间或参会人：`vc search`；多个候选时列出主题、时间与 `meeting_id` 让用户选，不要擅自取最近一场。
   - 只知道日历日程：`calendar agenda -o json` 取 `events[].event_id`（实例 ID），再用 `--calendar-event-ids`。
   - 会议正在进行：`vc meeting list-active`。
2. **读逐字稿**（先用 `vc notes --meeting-ids` 或 `vc note detail` 看纪要类型）
   - `note_display_type` 为 `normal`（`vc note detail` 中为 `1`）：`doc export <verbatim_doc> -o transcript.md`。
   - `unified`（`2`）：`vc note transcript <note_id>`。
   - 没有纪要但有妙记：`minutes get <minute_token> --transcript`（写文件）。
   - 两者都有且用户没指定：优先纪要逐字稿；用户明确说"妙记"时用妙记。
3. **AI 摘要/待办/章节/关键词**：`minutes get <minute_token> --summary --todo ...` 只取需要的项。
   `vc notes --with-artifacts` 会原样内联整份 AI 产物（含逐字稿全文，输出很大），且只对妙记路径生效。
   用户要现成总结时返回 AI 产物；要复盘、分析"谁说了什么"时读逐字稿原文。
4. **会中与会后**：进行中的会议用 `vc meeting list-active` + `vc bot meeting-events`；会议结束后不要再拉会中事件
   （实测返回 120002/120003），改用 `vc detail` / `vc notes` 读会后产物。会议刚结束时纪要和妙记可能还在生成。
5. **需要用户同意的操作**：`minutes apply-permission` 会通知妙记所有者；`meeting-join` / `meeting-leave`
   会让机器人进出会议、参会人可见。执行前说明影响并征得同意；验证参数只用 `--dry-run`。

## 命令：搜索与定位会议

### vc search

```bash
feishu-cli vc search --query "周会" --start 2026-03-20 --end 2026-04-11
feishu-cli vc search --organizer-ids ou_xxx,ou_yyy -o json
feishu-cli vc search --query 123456789        # 按 9 位会议号搜索
```

- 至少指定 `--query/--start/--end/--organizer-ids/--participant-ids/--room-ids` 之一；`--query` 最长 50 字符。
- `--organizer-ids/--participant-ids/--room-ids` 逗号分隔、各最多 50 个，只接受 open_id，不支持 `me`
  （传 `me` 实测报 99992351；这点与 `minutes search` 不同）。
- `--page-size` 1-30（默认 15），有 `has_more` 时用返回的 `--page-token` 翻页。
- 时间接受 `YYYY-MM-DD`、`YYYY-MM-DD HH:MM[:SS]`、RFC3339；不带时区的按本地时区解析，纯日期的 `--end` 对齐到 23:59:59。
- `-o json` 原样透传：`items[].id`（即 `meeting_id`）、`display_info`、`meta_data.description`、`has_more`、`page_token`。

### vc detail

```bash
feishu-cli vc detail 6911188411932033028 -o json
feishu-cli vc detail 123456789 --start 2026-06-01 --end 2026-07-22
```

- 位置参数是 `meeting_id`，或 9 位会议号。会议号会先在时间窗口内反查关联会议（默认近 90 天，可用 `--start/--end` 调整），
  周期性会议可能对应多场，分别输出。
- `-o json` 结构：`{meetings: [...], meeting_no}`（`meeting_no` 只在按会议号查询时出现）；每项含
  `meeting_id / meeting_no / topic / start_time / end_time / status / note_id / minute_token / hint / error`。
- `status=ongoing` 时纪要与妙记尚未生成，相关字段为空并写入 `hint`；`ended` 时补查录制拿 `minute_token`。
  缺 `note_id` 或 `minute_token` 只写 `hint`，全部会议都查询失败时才以非 0 退出。

### vc recording

```bash
feishu-cli vc recording --meeting-ids 6900001,6900002 -o json
feishu-cli vc recording --calendar-event-ids <event_id> -o json
```

- `--meeting-ids` 与 `--calendar-event-ids` 互斥，均最多 50 条。
- `-o json` 结构：`{items: [{id, ok, data: {meeting_id, minute_token, recording_url, duration}, error}], summary}`，
  没有 `status` 字段；`duration` 为毫秒。会议没有录制时该项失败（`121004 data not exist`），部分成功时退出码仍为 0。

## 命令：纪要与逐字稿

### vc notes

```bash
feishu-cli vc notes --meeting-ids 6900001,6900002 -o json
feishu-cli vc notes --minute-tokens obcnxxxx --with-artifacts --download-transcript --output-dir ./notes -o json
feishu-cli vc notes --calendar-event-ids <event_id> -o json
```

- `--meeting-ids` / `--minute-tokens` / `--calendar-event-ids` 三者互斥，均最多 50 条。
- `-o json` 结构：`{items, summary}`，`items[]` 为 `{id, ok, data, error}`；日历路径的 `data` 是数组
  （一个日程可能关联多场会议或多个妙记）。`data` 字段：`source / meeting_id / minute_token / note_id /
  note_display_type / title / minute_url / create_time / note_doc / verbatim_doc / shared_docs / artifacts /
  transcript_path / hint`。
- `note_display_type`：`normal`（逐字稿在 `verbatim_doc` 文档里）/ `unified`（用 `vc note transcript`）/ `unknown`。
- 会议路径总会尝试经录制补 `minute_token`；没有纪要、纪要无权限（121005）、没有录制（121004）都写入 `hint`，不中断整批。
- **`--with-artifacts` 与 `--download-transcript` 只作用于妙记路径**：`--minute-tokens`，或日历路径中没被会议路径覆盖的妙记。
  `--meeting-ids` 和常见的日历路径会静默忽略这两个开关（实测无 `artifacts`、无 `transcript_path`）——先拿到
  `minute_token`，再用 `--minute-tokens` 或 `minutes get`。
- `--with-artifacts` 原样输出 AI 产物接口的数据：实测包含 `keywords` 和逐字稿全文 `transcript`；没生成的
  `summary` 等字段可能缺失。
- `--download-transcript` 写入 `{output-dir}/artifact-{标题}-{token}/transcript.txt`（带说话人和时间戳的纯文本），
  `--output-dir` 默认当前目录；同一次调用中同一 `minute_token` 只下载一次。文件已存在且未加 `--overwrite` 时，
  该项 `transcript_path` 变成 `下载失败: 文件已存在...`，`ok` 仍为 true、退出码为 0——要检查 `transcript_path` 的值。

### vc note（按 note_id 操作单篇纪要）

```bash
feishu-cli vc note detail <note_id>
feishu-cli vc note transcript <note_id> --output transcript.md
feishu-cli vc note transcript <note_id> --format plain_text --locale en_us --output transcript.txt
```

- `detail` 固定输出 JSON（没有 `-o`）：`note.note_display_type`（1=普通纪要，2=统一纪要）、`note.artifacts[]`
  （`artifact_type=1` 是纪要主文档，`artifact_type=2` 是逐字稿文档，取 `doc_token`）、`note.references[]`（共享文档）。
- `transcript` 只支持 User，先查纪要类型：普通纪要（或服务端返回 `121002 not support`）直接以 exit 2 报错，
  错误信息给出 `doc export <verbatim_doc_token>` 命令。
- `--format` 取 `markdown`（默认）或 `plain_text`；旧值 `text` 按 `plain_text` 处理并在 stderr 提示。
  `--locale` 缺省时飞书品牌为 `zh_cn`、Lark 品牌为 `en_us`。
- 自动翻页拉全量；任一页失败、游标不前进、超过 500 页或结果为空都会报错，不会输出半截逐字稿。
- `--output` 写文件且会直接覆盖已有文件；不传时打印到 stdout。`121005` 表示当前身份无该纪要阅读权限（exit 3）。

## 命令：妙记

### minutes search

```bash
feishu-cli minutes search --query "预算复盘" --owner-ids me -o json
feishu-cli minutes search --participant-ids me --start 2026-03-01
```

- 至少指定 `--query/--owner-ids/--participant-ids/--start/--end` 之一；`--keyword` 是 `--query` 的别名。
- `--owner-ids/--participant-ids` 逗号分隔、任一匹配，值必须是 `ou_` 开头的 open_id 或 `me`；`me` 需要已登录 User 才能解析。
- 时间按妙记创建时间过滤，格式同 `vc search`。结果固定按创建时间倒序，`--page-token` 翻页不漏条、不重复。
- 旧参数 `--owner-id` / `--start-time` / `--end-time` 仍可用，stderr 会提示已废弃；新旧同时传不同值时报用法错误。
- `-o json` 透传 `items[].token / display_info / meta_data / has_more / page_token`；`display_info` 含转义的高亮标签，
  文本模式会剥掉标签并取首行作标题。

### minutes get

```bash
feishu-cli minutes get obcnxxxx --summary --todo -o json
feishu-cli minutes get obcnxxxx --transcript --output-dir ./notes
feishu-cli minutes get obcnxxxx --wait-ready --wait-timeout 600
```

- 只选需要的产物：`--summary`（`summary`）、`--todo`（`minute_todos`）、`--chapter`（`minute_chapters`）、
  `--keyword`（`keywords`）、`--transcript`。一个都不选时不调用 AI 产物接口。`--with-artifacts` 是兼容旧用法，等于全选。
- `--transcript` 把逐字稿写文件，输出只给 `transcript_file` 路径：默认 `./minutes/<token>/transcript.txt`；
  指定 `--output-dir` 时为 `<dir>/artifact-<标题>-<token>/transcript.txt`。文件已存在且未加 `--overwrite` 时保留原文件、
  返回原路径并在 stderr 提示。逐字稿为空时 `transcript_file` 为空串。
- `-o json` 结构：`{minute, artifacts}`。`minute` 是原始接口数据，标题等字段在 `minute.minute.title / url /
  create_time / duration / owner_id`（时间与时长为毫秒）；`artifacts` 只含选中的键。
- AI 产物接口失败时，JSON 输出 `{minute, artifacts_error}`、文本模式打印失败原因，退出码仍为 0——要检查 `artifacts_error`。
- `--wait-ready` 只在妙记或 AI 产物仍在生成（`2091003`）时按 `--wait-interval`（默认 10 秒）轮询，最长 `--wait-timeout`
  （默认 300 秒）；已就绪时立即返回，适合刚结束的会议。
- 无权限（`2091005`，exit 3）时错误信息会提示 `minutes apply-permission`。

### minutes download

```bash
feishu-cli minutes download --minute-tokens obcnxxxx
feishu-cli minutes download --minute-tokens t1,t2,t3 --output ./media
feishu-cli minutes download --minute-tokens obcnxxxx --url-only
```

- `--minute-tokens` 必填，最多 50 条；批量请求自动限速。
- `-o/--output` 是**保存路径**，不是输出格式（本命令没有 JSON 输出，`-o json` 会把文件存成名为 `json` 的文件）。
  单个 token 时可以是文件路径或目录，多个 token 时必须是目录；默认当前目录。
- 文件名优先取服务端 Content-Disposition，其次按 Content-Type 推导扩展名，兜底 `<token>.media`。目标文件已存在时该项失败，
  加 `--overwrite` 才覆盖。
- 批量下载时不要依赖同名去重：源码中同名检测发生在写盘之后，未加 `--overwrite` 时后一个文件会因"文件已存在"失败，
  加了 `--overwrite` 则会先覆盖前一个文件再改名为 `<token>-<文件名>`。可能重名时逐个下载并用 `--output <文件路径>` 指定文件名。
- `--url-only` 只打印预签名下载链接，不落盘。部分失败时退出码仍为 0，全部失败才非 0。

### minutes apply-permission

```bash
feishu-cli minutes apply-permission --minute-token obcnxxxx --perm view
```

- `--perm` 取 `view` 或 `edit`。申请会通知妙记所有者，执行前先征得用户同意；没有 `--dry-run`。

## 命令：进行中的会议与会中事件

### vc meeting list-active

```bash
feishu-cli vc meeting list-active -o json
feishu-cli vc meeting list-active --as bot --user-id ou_xxx -o json
```

- 返回 `meetings[].meeting_id / meeting_no / meeting_title`；没有进行中的会议时 `meetings` 为空数组。
- `--as user`（默认）查当前登录用户；`--as bot` 必须带 `--user-id`（`ou_` 开头），`--as auto` 回退到 Bot 时同样需要。
  官方 CLI 文档说明 Bot 身份只返回"目标用户在会中且应用机器人也在同一会议中"的会议，因此 Bot 返回空不代表用户没在开会（未实测）。
- 同时在多个会议中时，先让用户确认 `meeting_id`，再查事件。支持 `--dry-run`。

### vc bot meeting-events

```bash
feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --as user --page-all -o json
feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 \
  --as user --start 2026-03-01 --end 2026-03-31 --page-size 50 -o json
feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --as bot --dry-run
```

- `--meeting-id` 必须是长数字 `meeting_id`；传 9 位会议号直接以 exit 2 拒绝，错误信息提示用 `vc meeting list-active`
  或 `vc detail <会议号>` 换取。
- 身份按 `meeting_id` 来源选：用户发现（`list-active` 默认身份）用 `--as user`；机器人入会或 `list-active --as bot`
  得到的用 `--as bot`（机器人必须在会中）。`--as user` 缺 Token 直接失败，`--as bot` 即使已登录也走 Bot。
- `--start/--end` 接受 `YYYY-MM-DD`、RFC3339 或 Unix 秒；`--start` 晚于 `--end` 报用法错误。
- `--page-size` 取值 20-100（默认 20，传 0 等同默认），1-19 或大于 100 都会被拒；`--page-all` 固定每页 100、最多 200 页，
  遇到空或重复游标即停止，合并后的输出保留末页 `has_more / page_token`，达到页数上限时 stderr 告警。
- 事件列表字段是 `events`，`-o json` 原样透传 `events / has_more / page_token`；文本模式逐条打印时间、类型和事件原文。
  单页结果有 `has_more` 时只是部分事件，用 `--page-token` 续拉或改用 `--page-all`。
- 回答"会上刚刚发生了什么"时重新拉取最新事件，不复用旧结果。

## 命令：会议机器人入会/离会

```bash
feishu-cli vc bot meeting-join --meeting-number 123456789 --password 1234 --dry-run
feishu-cli vc bot meeting-join --meeting-number 123456789 --call-id <call_id> --action start --dry-run
feishu-cli vc bot meeting-leave --meeting-id 6911188411932033028 --dry-run
```

- 只支持 Bot（见[身份与前置检查](#身份与前置检查)），需要应用开通 `vc:meeting.bot.join:write`，不需要 User 登录。
- `meeting-join --meeting-number` 必须是 9 位数字；`--password` 为会议密码；`--call-id` 透传邀请事件中的关联 ID；
  `--action join`（默认）加入会议，`--action start` 发起日程会议（请求体 `action=2`），其他取值报用法错误。
- `meeting-leave --meeting-id` 用长数字 `meeting_id`（入会返回结果或 `list-active` 中获取）。
- `--dry-run` 只打印请求方法、路径、请求体和 `"as": "bot"`，不调用接口；`-o json` 输出原始响应。
- 入会与离会对参会人可见，真实执行前必须得到用户明确同意。

## 典型工作流

### A：历史会议 → 纪要与逐字稿

```bash
# 1. 定位会议，记录 items[].id（meeting_id）
feishu-cli vc search --query "架构评审" --start 2026-03-01 -o json

# 2. 查纪要类型、文档 token 与 minute_token
feishu-cli vc notes --meeting-ids <meeting_id> -o json
# → items[].data.note_display_type / note_id / verbatim_doc / minute_token

# 3a. normal：导出 verbatim 逐字稿文档
feishu-cli doc export <verbatim_doc> -o transcript.md
# 3b. unified：导出统一逐字稿
feishu-cli vc note transcript <note_id> --output transcript.md
# 3c. 只有妙记：逐字稿写文件
feishu-cli minutes get <minute_token> --transcript
```

### B：日历日程 → 妙记 AI 产物与音视频

```bash
# 1. 取日程实例 ID（agenda 输出的 event_id）
feishu-cli calendar agenda --start-date 2026-03-20 -o json

# 2. 反查会议、纪要与 minute_token（AI 产物和逐字稿在第 3 步按 minute_token 获取）
feishu-cli vc notes --calendar-event-ids <event_id> -o json

# 3. 按 minute_token 取 AI 产物与逐字稿
feishu-cli minutes get <minute_token> --summary --todo --chapter --transcript -o json

# 4. 需要原始音视频时下载
feishu-cli minutes download --minute-tokens <minute_token> --output ./media
```

### C：进行中的会议 → 会中事件

```bash
# 1. 找到当前会议（默认 User 身份）
feishu-cli vc meeting list-active -o json

# 2. 沿用同一身份拉全量事件
feishu-cli vc bot meeting-events --meeting-id <meeting_id> --as user --page-all -o json
```

## 权限要求

以下为最小 scope；项目顶层 CLAUDE.md 用 `minutes:minutes*:*` 通配概括妙记相关 scope。

| 命令 / 功能 | 必需 scope |
|---|---|
| `vc search` | `vc:meeting.search:read` |
| `vc detail`（meeting_id） | `vc:meeting.meetingevent:read`、`vc:record:readonly` |
| `vc detail`（9 位会议号） | 额外 `vc:meeting:readonly` 或 `vc:meeting.meetingid:read`（缺失实测报 99991679） |
| `vc recording` | `vc:record:readonly`（日历路径另需 `calendar:calendar:read`、`calendar:calendar.event:read`） |
| `vc notes`（meeting-ids） | `vc:meeting.meetingevent:read`、`vc:note:read`；补 `minute_token` 用 `vc:record:readonly`（失败只写 hint） |
| `vc notes`（minute-tokens） | `minutes:minutes:readonly`、`vc:note:read` |
| `vc notes --with-artifacts` | + `minutes:minutes.artifacts:read` |
| `vc notes --download-transcript` | + `minutes:minutes.transcript:export` |
| `vc notes`（calendar-event-ids） | + `calendar:calendar:read`、`calendar:calendar.event:read` |
| `vc note detail` / `vc note transcript` | `vc:note:read` |
| `minutes search` | `minutes:minutes.search:read` |
| `minutes get` | `minutes:minutes:readonly`；选任一 AI 产物（含 `--transcript`）或 `--with-artifacts` 另需 `minutes:minutes.artifacts:read` |
| `minutes download` | `minutes:minutes.media:export` |
| `minutes apply-permission` | `minutes:permission:apply` |
| `vc meeting list-active` / `vc bot meeting-events` | User：`vc:meeting.meetingevent:read`；Bot：应用开通 `vc:meeting.meetingevent:read` 或 `vc:meeting.bot.join:write` 任一（99991672 提示实测） |
| `vc bot meeting-join` / `meeting-leave` | 应用开通 `vc:meeting.bot.join:write` |

User 路径缺 scope 时执行 `feishu-cli auth login --scope "<所需 scope>"`，或 `feishu-cli auth login --domain vc --domain minutes --recommend`；
Bot 路径的 scope 在飞书开放平台的应用权限管理页面开通并发布版本，重新登录无法解决。

## 坑点与错误处理

| 错误码 / 现象 | 含义 | 处理 |
|---|---|---|
| `99991679` | User Token 缺 scope | 按错误提示 `auth login --scope "..."` 补授权 |
| `99991672` | 应用未开通所需 scope（常见于 `--as bot`） | 在开放平台为应用开通；不要反复 `auth login` |
| `99992351` | ID 格式不合法（如给 `vc search` 传 `me`） | 改传 `ou_` 开头的 open_id |
| `2091003` | 妙记或 AI 产物仍在生成 | `minutes get --wait-ready`，或稍后重试 |
| `2091005` | 当前身份无妙记阅读权限（exit 3） | 征得用户同意后 `minutes apply-permission --minute-token <token> --perm view` |
| `121002` | 普通纪要不支持统一逐字稿 | 改用 `doc export <verbatim_doc>` |
| `121004` | 会议没有录制（`data not exist`） | 该会议没有妙记；改读纪要 |
| `121005` | 无纪要阅读权限（`vc note` 为 exit 3；`vc notes` 写入 hint） | 联系纪要所有者授权 |
| `120002` | 读会中事件被拒：主持人未开启 AI 纪要或未允许 agent 入会 | 请主持人开启后重试；会议已结束时改读会后产物 |
| `120003` | 当前身份不在该会议中（实测对已结束会议也返回） | 核对 `--as` 与 `meeting_id` 来源；会议已结束改用 `vc detail` / `vc notes` |

- 业务错误码随 HTTP 4xx 下发时也按业务码处理，错误信息附 `log_id` 便于排查；退出码：用法错误 2、鉴权或权限 3。
- 批量命令（`vc notes/recording`、`minutes download`）部分失败时退出码为 0，要逐项检查 `ok` / `error`。
- 所有逗号分隔的批量参数先去重，去重后超过 50 条直接报错；`minute_token` 前置校验为字母数字且至少 5 位。
- 刚结束的会议可能暂时查不到纪要或妙记；进行中的会议只能读会中事件。
