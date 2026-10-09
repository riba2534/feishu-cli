# Changelog

所有重要的项目变更都会记录在此文件。

版本格式：[MAJOR.MINOR.PATCH](https://semver.org/lang/zh-CN/)

## [Unreleased]

对照飞书官方 CLI（larksuite/cli）逐领域审查后的全面对齐：修复一批"代码自洽但与服务端契约不符"的缺陷，
补齐官方已有、本项目缺失的能力，并保留本项目更稳妥的设计（fail-closed 身份、host 白名单、本地转换器、画板全家桶等）。
所有修复均用编译后的二进制在真实飞书环境回归（写操作只针对新建的测试资源）。

### ⚠️ 行为变更（升级前请阅读）

- **退出码分类**：0 成功 / 1 一般错误 / 2 用法错误 / 3 鉴权或权限 / 4 网络 / 10 需要确认 / 130 中断。错误文本不变；脚本若判断 `== 1` 需调整为 `!= 0` 或按类别处理。`auth check` 未通过由 1 改为 3。
- **非交互删除必须带 `--yes`**：stdin 不是终端且未带 `--yes`（或命令级 `--force`）时，删除类操作以 10 退出且不执行。此前会打印"操作已取消"并 exit 0，Agent 会误判为删除成功。确认提示改写 stderr。
- **日历与任务写命令默认身份由 Bot 改为 auto**：`calendar create-event/update-event/delete-event/attendee add|remove` 与 task/tasklist 写命令新增 `--as`，已登录即以本人身份操作；要操作应用自己的资源请显式 `--as bot`。`calendar event-reply` 改为必须 User Token。
- **多维表格**：`record list` 默认返回 100 条（原 20 条且无提示），`has_more` 时输出 `next_offset` 并在 stderr 提示；`table/field/view list` 默认返回全量；workflow/form/dashboard list 未传 `--page-token` 时自动翻页；视图 group/sort/visible-fields 的 get/set 直接输出数组；`role` 输出去掉外层 `data`、`base_roles` 项由字符串变为对象；删除表单题目不带 `--keep-field` 时需要确认。
- **电子表格**：`delete-rows/delete-cols/protect` 的 `--start`（0 起始）/`--end`（不含）按文档语义换算为接口口径——此前 `--start N` 会多删一行、`protect` 范围错位；范围不带子表前缀且表格有多个子表时报用法错误。
- **文档**：`doc import` 部分失败（图片/表格/图表等）以 1 退出并在输出中列出失败明细；`content-update` 纯文本选择器改为文本级替换（不再覆盖整段）、块级模式多处命中报错、`partial_success` 非零退出；callout 背景色枚举修正（见"修复"）；`doc media-download` 默认不覆盖已有文件。
- **邮件**：`mail message/messages/thread` 的正文字段默认输出解码后的明文（`--raw-body` 保留原值）；`draft-edit` 改为局部修改；`triage --page-size` 语义改为总条数（自动翻页）；`message-modify/message-trash` 超过 20 封自动分批。
- **会议与妙记**：`vc bot meeting-join/leave` 仅支持 Bot（传 `--user-access-token` 报错）且会议号须为 9 位数字；`minutes get --with-artifacts` 的逐字稿改为写文件（`transcript_file`）。
- **云盘**：下载默认不再有 5 分钟总时长限制（改为 60 秒空闲超时）；`drive download --output <目录>` 文件名按 Content-Disposition → 标题 → token 决定；push/pull 遇缺 scope、无权限、限流、参数错误时终止整批；`file delete` 以异步方式提交并默认轮询；`file quota` 需要 User Token。
- **`api` 命令**：业务错误时 stdout 为空，错误与诊断写 stderr（`--raw` 仍原样输出响应体）。
- **认证**：`--scope` 与 `--domain/--recommend` 可叠加（原报错）；批量申请一律剔除 `im:message.send_as_user`；`--device-code` 续轮询上限 600 秒；token.json 可能新增 `refresh_failure` 标记。
- **OKR**：`okr cycle list` 默认查询 v2 用户周期（与 `cycle detail`、目标创建使用的 ID 一致），旧的租户周期用 `--tenant`。
- **IM**：`msg history` 在话题群只返回根消息（回复在 `thread_replies` 中，不再重复出现在 `items`）；`msg thread-messages` 的时间范围改为按秒在客户端过滤；`msg reaction/pin`、`chat get/update/delete` 新增 `--as`（默认 auto，未登录时可用 Bot）；`msg merge-forward` 固定使用应用身份；`chat member list` 新增 `users/bots/truncations` 字段（`items` 仍只含用户）；`user search --query` 改走 `contact/v3/users/search`，不再返回 `user_id`（需要时用 `--email/--mobile`）；`user search --email/--mobile` 的 `user_id` 字段改为真实 user_id（此前填的是 open_id）；`event consume` 同一应用单进程单连接并加单实例锁。
- **搜索文档**：`search docs` 改走官方使用的 Search v2 端点（`POST /open-apis/search/v2/doc_wiki/search`）：每页最多 20 条（原 50），翻页改用 `--page-token`，`--offset` 大于 0 时报用法错误；`--owner-ids`/`--chat-ids`/`--docs-types` 映射为 v2 filter，输出字段保持兼容并新增 `page_token`。
- **画板**：`board export-code --output-path` 目标文件已存在时默认报错，覆盖需加 `--overwrite`（此前静默覆盖）。
- **其他**：`mail message/messages/thread --format` 只接受服务端取值（full/plain_text_full/metadata）；`doctor --only`、`auth token` 参数冲突、`--page-limit` 越界、非法 `--as` 取值等改为用法错误（退出码 2）。

### 新增

**平台与工程**
- `feishu-cli skills list/read/install`：技能内嵌二进制、与 CLI 版本严格配套；安装时解析符号链接、记录 `.feishu-cli-skills.json`、保护本地修改，`--prune-legacy` 只清理确认是旧版 feishu-cli 技能的目录。
- `doctor` 新增 `skills` 检查（零网络检测本地技能与 CLI 版本漂移）；每个命令的 `--help` 末尾显示相关技能与工作流。
- `feishu-cli update [--check|--dry-run|--target]`：查询并安装最新 release（sha256 校验、自检后原子替换，不做后台检查）。
- 全局 `--yes`；`auth scopes`（应用已开通 scope 与逐项诊断）；`auth login --exclude`；授权域补齐 `okr`/`apps`/`markdown` 及命令实际所需 scope；`profile add`/`config init` 支持 `--app-secret-stdin` 与 `--probe`。
- 统一资源 URL 解析：只按路径前缀识别类型，`--type` 与 URL 冲突时报错；识别 `/drive/shr/`、`/chat/drive/`、`/spreadsheets/` 等；`drive inspect` 传裸 token 时自动识别类型。
- Bot 身份创建文档、文件夹、文件、表格、多维表格、知识库节点、演示文稿后，自动给当前 CLI 登录用户授予 `full_access`（输出 `permission_grant`）。
- 文档链接按品牌生成（www.feishu.cn / www.larksuite.com）。
- `make release-package VERSION=vX.Y.Z`、`make check-privacy`、GitHub Actions CI。

**文档**
- `content-update`：`--block-id`、`--start-block-id/--end-block-id`、`--mode str_replace`、`block_move_after`、`block_copy_insert_after`、`--doc-format xml`、本地图片/附件插入。
- `doc read --engine docs_ai`（`--with-ids`、`--scope outline|range|keyword|section`）；`doc create --content`（docs_ai 服务端建文档）；`doc export --engine docs_ai`；`doc history list|revert|revert-status`。
- 文档素材 >20MB 分片上传；`media-insert --width/--height`；`media-download --overwrite`。
- `doc get/blocks/add/add-board/add-callout/update/delete/batch-update/table`、`doc import --document-id`、`doc export-file` 接受文档或知识库链接；`doc import -o json` 新增 `file_*`、`whiteboard_*`、`blocks_failed` 统计。

**电子表格**
- `--as bot|user|auto`；表格参数接受 sheets/wiki URL；`--sheet-name`；`Sheet1!A1` 子表名前缀自动换算。
- `insert-cols`、`update-dimension`（隐藏、行高列宽）、`move-dimension`、`update-sheet`、`freeze`、`filter update`、`replace --regex`。
- `--range "3:5"`/`"B:D"`；删除类命令 `--dry-run`；`table-put --mode append/--start-cell`、自动扩容；图片素材 >20MB 分片上传。

**多维表格**
- `bitable resolve`（base/wiki/record/表单分享链接，`?table=` 判型）、`bitable block list`；`--base-token` 接受链接。
- `record list --page-all`；`bitable create --table-name/--fields`、`table create --fields`。
- 仪表盘块类型 ranking/nps 与 `--position`、`dashboard block get-data`；dashboard/form `share get/update`；附件 >20MB 分片上传。

**云盘与知识库**
- `drive update-title`、`drive version-history`、`drive version-get`；`task-result` 支持 `wiki_move/wiki_move_to_drive/wiki_delete_space`。
- push/pull `--if-exists smart`、`--on-duplicate-remote`、status `--quick`；`file list`/`wiki nodes`/`wiki spaces`/`comment list` 分页参数。
- `comment get/batch-get`、`comment reply update/react`；`drive add-comment` 支持 sheet/slides/bitable/file 锚点。
- perm 全部子命令 `--as` 与 `--user-access-token`；`perm add --perm-type`（wiki）。

**日历、任务、审批、OKR**
- 重复日程 `--apply-to single|all|this-and-following`；`calendar event-share`、`event-transfer`；`attendee remove`、`--room-ids`；`create-event --attendee-ids`（失败回滚）/`--vchat`；`freebusy` 多人与空闲视图。
- `approval task rollback/add-sign/remind`、`--page-all`；`okr objective/key-result create/update`、`okr comment list/create`；`task section`、`task related`、`task set-ancestor`、`tasklist search`。

**邮件、会议、妙记**
- `--attach`；`forward` 默认携带原附件；`triage --max` 自动翻页并补全摘要；`rule-*`（收信规则）、`thread-modify/thread-trash`、`template get/update/delete`。
- `vc meeting list-active`；会议/纪要/妙记读命令 `--as`；`minutes get --summary/--todo/--chapter/--keyword/--transcript`；`minutes search --participant-ids me`。

**Slides、画板、妙搭**
- `slides add-slide/delete-slide/replace-slide/update-slide/screenshot`；`slides create --slide`（带图片占位自动上传）；`slides get --slide-number/--output-file`。
- `apps get`、`apps release get/list`、`apps html-publish --wait`；`apps list --keyword`。
- `board export-code --source`；`board update/svg-import/import --client-token`。

**IM 与通讯录**
- `--text/--markdown/--content` 支持 `@文件` 与 `-`（stdin）；`--markdown` 发送前做样式归一（标题降级、表格补空行）；`--attachment` 附件区；音视频上传自动解析时长；`msg edit`；`msg send/reply` 输出 `chat_id/create_time` 并支持 `--dry-run`。
- `chat create --chat-mode topic --bots`（返回分享链接）；`chat list --types p2p --exclude-muted`；`msg search-chats --member-ids/--chat-modes/--sort/--exclude-muted`。
- `user search` 关键词搜索支持 `--has-chatted` 等过滤；`user search-bot`；`user info --as user`。
- `event consume` 按 event_id 去重，启动前探测远端已有连接。

### 修复

**数据安全**
- `drive push --if-exists overwrite` 改为携带 file_token 原地覆盖：file_token、链接、协作者、评论、历史版本保留；此前先删后传，删除成功而上传失败时文件丢失。
- `sheet delete-rows/delete-cols` 索引口径错误导致多删一行；`protect` 范围错位。
- `doc content-update` 纯文本选择器整块替换导致段落其余文字丢失；结束锚点取最后一次出现可能删到文末；本地导出方言（画板占位、callout、图片 token）写回 docs_ai 会静默丢内容——现在转换或拒绝。
- 删除表单题目默认连带删除整列数据却无提示。
- `apps html-publish` 把 `.git` 目录打包发布到公网。
- 输出/输入路径拒绝敏感目录（~/.ssh、~/.aws、~/.feishu-cli、/etc 等），不再误拒 `report..v2.json`。
- 建块与画板写入的自动重试复用幂等 token（或回读确认），避免 5xx 重放产生重复块/重复图。

**与服务端契约不符**
- 知识库节点解析改用 node_by_token（obj_token、文档 URL 可直接解析，错误码分类提示）。
- 业务错误随 HTTP 400 下发时先解析业务码：错误信息附带 log_id、缺失 scope、字段校验；99991672 不再提示重新登录，改为给出开放平台开通链接。
- `api --data` 中的大整数（19 位 ID）按原始字面量发送，不再经 float64 舍入；电子表格写入同样保真。
- 导入型 Office token 判定（27 位格式）修正，图片上传后可正常渲染。
- callout 背景色枚举修正为 1 红 / 2 橙 / 3 黄 / 4 绿 / 5 蓝 / 6 紫 / 7 灰（此前整体错一位，NOTE 显示为浅紫）。
- 多维表格：批量删除上限 200（原写 500）并自动分批；create/copy 文本输出正确打印 token；视图配置解包；字段列表跨页顺序不稳定时按 id 去重取全；限流 800004135 自动重试。
- 电子表格：数字不再出现 `1e+06`；`table-put` 保留时分秒、空串写空单元格、数值列重置残留的文本格式；`write/append` 超过 5000 行自动分批；`filter create` 补齐 col 与 condition。
- 云盘：Bot 下载流式化（无 100MB 上限、内存占用恒定），分片失败有界重试并断点续传、原子写；删除任务的 `fail` 状态识别为失败；导出/导入轮询容忍瞬时错误；评论列表输出正文、回复保留 @人与链接；`perm public-get` 改用 v2；异步删除失败时按身份提示原因。
- 日历：全天日程起止不再为空；`freebusy` 默认当前用户并合并区间；重复日程删除/修改说明影响范围（修正帮助与文档的矛盾）。
- 任务与审批：`task complete` 幂等；完成状态 flag 互斥；中文按字符截断；审批稀疏分页空页不再误报"没有找到"。
- OKR：周期列表与目标操作使用同一套 v2 周期 ID。
- 邮件：正文 base64url 解码；HTML 引用块转义；`In-Reply-To/References` 带尖括号并写 `X-LMS-Reply-To-Message-Id`；回复优先 Reply-To、支持回复自己发出的邮件；显示名按 RFC 5322 编码；`draft-edit` 不再丢回复头与附件；`triage --folder inbox` 不再 4038。
- 会议与妙记：`vc note transcript` 按统一逐字稿协议重写；`meeting-events` 文本模式改读 `events`（原永远 0 条）；妙记搜索固定按创建时间倒序（翻页不再漏条）。
- 画板与 Slides：`board import --syntax svg` 不再被当作 PlantUML（改走服务端 SVG 解析，生成可编辑节点）；空画板 `delete --all` 不再报错；`slides get` 接受 URL，`--revision-id 0` 明确拒绝。
- 认证：scope 支持逗号分隔；`auth status --verify` 改为加锁刷新并校验 App 绑定；刷新失败按错误码分类（终态记录标记，不再每条命令重复刷新）；时间输出使用真实时区（RFC3339）；`create-app` 对齐注册协议（`expire_in`、Lark 租户品牌）。
- 运行时：限流等待只向上抖动并支持 Retry-After；手写请求统一走共享连接池与受控客户端；Ctrl-C 可中断进行中的请求；拼错子命令给出建议；错误预览与落盘文件名按 UTF-8 字符边界截断。
- IM：话题群 `msg history` 线程回复重复输出、翻页被回复占用；`--markdown/--content` 的 @ 标签未规范化导致 @ 失效；`chat member list` 拿不到群内机器人；事件订阅按事件类型各开一条连接、与同应用其他连接争抢事件；设了 `--timeout/--max-events` 时 stdin EOF 仍提前退出；`msg history` 文本模式也全量拉取群成员；User 身份降级到搜索时提示语与身份不符且静默丢弃时间参数；`--user-email` 模糊匹配可能定位到错误的人；`search messages/chats` 丢弃服务端 notice；资源下载默认文件名改为服务端文件名。
- 集成阶段：SDK token 缓存改为随 client 实例重建而丢弃（同一 app_id 切换 base_url 或轮换 secret 后不再复用旧 token）；异步删除任务失败时按身份提示原因；`table-put` 数值列写入前重置残留的文本格式。
- 文档导入往返：`doc export` 输出的 `<image/file token>`、`feishu://media/` 视频导入时下载原素材重新上传，`<whiteboard token>` 按源码重建或复制节点（不再整篇报 1770001 失败），带 token 的 `<sheet>`/`<bitable>` 降级为链接并计入 `failures`；嵌套引用扁平化（原 1770030）；分栏内容写入服务端生成的列（原 1770028），列内空行不再截断；列表项的后续段落保留为子块；表格单元格里的 `$..$` 转为公式；被服务端拒绝的单个块隔离跳过，建块阶段失败也输出 JSON（退出码 1）；`doc export --download-images -o` 的资源路径相对输出文件，可原地再导入；`markdown patch` 读不到远端文件名时拒绝写回（新增 `--name`），参数错误退出码 2。
- 技能 review 阶段：`msg read-users` 已登录时必然失败（接口只收 Tenant）改用原始请求并新增 `--as`；`file version create/get/list` 兼容数字 status，避免创建成功后误报失败而重复建版本；妙记命令接受妙记链接，`vc notes` 在会议号/日历路径下也补拉纪要产物，产物部分失败时非零退出，`minutes download` 同批同名文件自动改名；邮件内联图片在 home 为软链接时不再被误拒；`--domain mail --recommend` 补收信规则 scope、slides 授权域补齐子命令 scope；`slides create` 按服务端返回统计页数；`calendar attendee remove` 的 dry-run 与真实请求一致；`okr comment create --select-all` 配 `--content-json` 时选区不再为空；`perm password` 遇 1063002 提示先开放链接；多处 help 与错误提示修正。

### 技能与验证

- 9 个领域技能同步上述行为变化，修正实测证伪的文档结论（bitable 视图配置 schema、data-query DSL、OKR "v2/cycles 不存在"、日历删除语义、考勤与任务示例、`Sheet1!` 写法、权限命令身份等）；各领域 SKILL.md 增加鉴权/scope 错误指引。
- 新增 CI（gofmt / vet / test / check-skills / check-privacy）；隐私扫描接入 `make check-privacy`。
- 技能目录全面 review：逐条按新二进制 `--help`、源码与实跑核对 9 个技能；新增 Agent 调用契约（退出码、确认门禁、stdout/stderr、自动授权、目标解析）；`msg`/`chat`、`drive` → `comment`/`perm`/`search` 的内容按命令归属迁移，跨工作流命令组要求显式前缀并在 `--help` 列出全部相关工作流；身份表按 `--help` 核对；9 个 description 改为"能力 → 何时用 → 不用于"单行结构，trigger-evals 替换 17 条、boundary evals 新增 11 条，evals.json 为新增命令组补 21 条用例。**本次未运行模型触发评测**，改为静态核对触发词覆盖。

## [v1.41.0] - 2026-09-22

### 新增

- `sheet image write-batch`：新增原生单元格图片批量写入命令，支持通过 manifest（文件、stdin `-` 或行内 JSON）并发下载、串行写入并经由 V3 `read-rich` 回读校验，内置连接池复用与全抖动限流重试。
- `sheet image write-image`：升级单格图片写入命令，支持直接传入 HTTPS 网络图片 URL 与 `-o json` 输出，写入后自动通过 V3 `read-rich` 回读验证原生 `image_token`。强化范围校验，若传入的范围前缀与目标 `sheet_id` 不一致时实行 fail-fast 拦截报错（原先为静默尊重前缀但会导致回读验证失配）。
- `skills/feishu-cli-data`：更新技能说明与表格工作流规范，明确单元格原生图片写入规则，严禁使用 `=IMAGE(...)` 公式或 Markdown 图片语法替代。

### 修复

- 单元格图片批处理遵守同一文档串行写入要求，下载与预处理仍可并发。
- 图片文件名根据实际格式补齐有效后缀，兼容无后缀 URL、动态图片地址及本地文件。
- BMP/TIFF/WebP 在写入前自动转为 PNG，兼容服务端对图片扩展名的限制；恢复 HEIC/BPG 的格式识别及原样提交能力，结果仍以服务端支持为准。补充批量写入、回读、取消与部分失败回归测试。

### 技能与验证

- 统一 9 个领域 Skill 的标准元数据、依赖说明和触发边界，补充跨领域与不应触发的评测；集中维护 User/Bot 身份规则，修正邮件、会议、审批与通用 API 的预检说明。
- 聊天导出在命令失败、分页截断或游标异常时明确失败，去重消息并保留已解析的机器人名称，避免将不完整历史报告为完整结果。
- SVG 转画板支持非零及负数 viewBox 原点，上传数量异常或回读未完成时不再报告成功；HTMLBox 验证使用独立浏览器会话，检查命令结果、页面错误和新生成的有效截图，失败时返回非零状态。
- 修正文档图片更新、日历参会人、考勤日期、多维表格字段和大文件导入示例；根据生效配置读取 owner，不再默认追加通知或将本地可视化任务发布到飞书。
- `make check-skills` 增加真实 YAML 解析、当前编译二进制的示例参数校验、领域脚本回归及本地 API 契约测试，区分静态校验、模拟请求与线上验证。

## [v1.40.0] - 2026-08-28

### 修复 — 深度 review：数据破坏、功能失效与静默降级（23 项）

对分支全量改动做分域深审并逐条用编译二进制真实调用飞书 API 验证。以下缺陷均**无法**被
`gofmt`/`go vet`/`go test`/`-race` 捕获（基线本来全绿），属"代码自洽但与服务端契约不符"。

**数据破坏（原实现 exit 0 静默发生）**

- `doc content-update --mode replace_all`：无 `#` 的模糊标题选择器同时命中父标题与其子标题时，
  外层范围会吞掉内层与其后未匹配的兄弟章节（实测 7 块文档替换后只剩 2 块，无关章节被销毁，
  却报告"成功替换 2 处"）。现 fail-closed 要求用带级别选择器或 `replace_range`。
- `drive pull --delete-local` / `push --delete-remote`：身份静默降级 Bot 后远端视图更小、差集更大，
  会把本地文件当作"远端已不存在"删除。改用 `resolveOptionalUserTokenForDestructive` fail-closed。
- `event` last-consumer 注销：`unsubscription` 在文件锁外执行，会抹掉窗口期内新 consumer 刚建立的
  订阅（新 consumer 已 ready 却静默收不到事件）。注销后复检存活 consumer 数并幂等补订阅。
- `calendar agenda`：午夜发生 DST 跳变的时区（如 America/Sao_Paulo 2018-11-04）区间倒挂
  （实测 `dur=-1s`），静默返回空结果。start/end 改用日历日期分量计算。
- `wiki delete`：确认提示未提级联范围（`--include-children` 默认 true），用户以为删单节点实际销毁整棵子树。
  现按实际范围提示；JSON 的 `ready`/`failed` 改用任务终态判定（不再恒为 `ready=true`）。

**功能完全失效**

- `mail triage`：`page_size` 是该端点必填参数，条件发送导致命令 100% 失败（99992402）。
  现始终发送并按端点上限截断（list 20 / search 15，对齐官方 shortcuts/mail）。
- `schema` / `api` catalog overlay：此前**完全不生效**（`source` 恒为 `embedded`、cache 目录为空）。
  三重原因：① 版本门禁要求严格更新，而官方顶层 `version` 恒为 `1.0.0`；② 短命 CLI 进程中后台刷新
  goroutine 被杀；③ 传 `data_version` 触发条件请求返回 `data:{}`。修复后 **12 service/152 method →
  15 service/250 method**，首启约 190ms 后走 cache，新增 `FEISHU_CLI_META_FIRST_SYNC_MS` 可调预算。
- `sheet write/append/prepend/batch`：删除 bool→`"TRUE"`/`"FALSE"` 转换后，v2 API 拒绝 JSON Boolean
  （实测 `code=90204 invalid cell type, type is bool`），含布尔值的写入全部失败。已恢复转换（官方
  `stringifyCellValue` 同样如此），4 条写入路径统一处理。
- `sheet protect` / `unprotect`：被误判「官方已废弃且无替代」而整体禁用并隐藏，实测两端点均返回
  `code=0` 可用。恢复实现，并修正 `protectId` 解析层级（在 `addProtectedDimension[i]` 顶层，非嵌套 `dimension` 内）。
- `calendar agenda` 长区间：193103/193104 随 HTTP 400 下发，而 `StatusCode != 200` 提前返回短路了
  自动切分恢复逻辑。修复后 90 天窗口从直接失败变为返回 334 个日程。

**安全**

- `markdown` 取 tenant token 用裸 `http.Post`（`http.DefaultClient`），绕过本分支新增的重定向/凭证
  策略层——`app_secret` 可随 3xx 重放到任意 host 且无超时。改走 `auth.FetchTenantAccessTokenResult`。
  同类问题统一修 4 处（markdown preview_download、drive download、message resource、event subscribe），
  全部改用 `config.NewHTTPClient`（host 校验 + 重定向剥离 Authorization）。
- `wiki delete` 的 HTTP loopback 豁免用 `strings.HasPrefix(hostname, "127.0.0.")`，会把攻击者可注册的
  `127.0.0.evil.com` 当本地地址放行。改用 `net.ParseIP` 精确判定。
- `multipart_session`：`block_size` 守卫以 `maxNativeInt64()` 为上限，在 64 位平台等于钳制值而永久
  失效，服务端返回 `1<<62` 会让 `make([]byte)` panic。加 64MB 合理上限；原测试因 `t.Skip` 在 64 位
  平台从不执行，改为全平台有效。

**静默失败 / 契约不符**

- `internal/client/timeparse`：13 位毫秒时间戳被当秒解析（得到公元 56971 年）。现按数量级识别。
- `api_code`：正则从 `code=N` 放宽后会命中 `status code: 500`，使永久 4xx 被 `IsRetryableError`
  判成可重试。收紧匹配并对 4xx 前置判定（保留 `task.go` 的 `(code: N)` 形态）。
- `api --page-all`：`has_more` 类型严格断言致静默截断（现容忍 bool/数字/字符串）；`page_token` 为空
  时不再中止翻页而回落 `next_page_token`。
- `api --params`：流式 decoder 静默丢弃尾部残留（`'{"a":1} {"b":2}'` 只取前半）。现显式报错。
- `doc content-update`：`<!-- feishu-colwidth: ... -->` 指令被静默丢弃（此前只拦 flag）。两条入口都 fail-closed。
- `markdown overwrite`：取不到远端名时回落 `<token>.md`、`--content-file` 时用本地文件名，两条路径
  都会静默重命名远端文件。现缺省一律读远端现有名，读不到则报错。
- 读类命令身份降级：`resolveOptionalUserTokenWithFallback` 把所有错误静默吞掉（含新增的 app_id 绑定
  守卫），与 `--as` 类命令 fail-closed 的行为自相矛盾。现在 stderr 明确告警（stdout 不受影响）。
- `approval task query --topic started`：topic=3 已被官方下线（服务端仅接受 1/2/17/18），
  前置报错并指向 `approval instance initiated`。
- `approval:approval:read`（`approval get` 实测必需）不在官方 scope 快照中，`--recommend` 会少授权，
  在 `scope_overrides.json` 显式放行。
- `drive export` 原子写入补齐 Chmod / 目录 fsync / Windows 覆盖兜底（与 `internal/auth` 版本对齐）。
- `internal/registry` 生产文件曾 `import "testing"` 并按 `testing.Testing()` 分支，改为注入式 seam。

**文档同步**：CLAUDE.md（Token 策略五 helper、列宽适用范围、审批 topic、catalog 数字）、
11 处 skill 工作流文档、`attendance user-stats query --user-ids` 必填示例。

### 修复 / 协议对齐 — native Markdown 与 Drive import/export/move

- **Markdown 源/历史下载**改为 `GET /open-apis/drive/v1/medias/{token}/preview_download?preview_type=16`，支持 `--version`。
- 新增 `markdown patch`；`create` 支持 `--wiki-token`；`create/overwrite/patch` 在 **20MB+1** 走 `files/upload_prepare/part/finish`，覆盖保留 `file_token`。
- Markdown 与 Drive import/export/export-download/move 增加 `--as bot|user|auto`（默认 auto；已配置 User 但刷新失败 fail-closed；`--dry-run` 不解析 token）。依赖已验收的 `fix(api): generic api 业务错误码校验与 auto 身份 fail-closed`。
- **Drive import**：`medias/upload_all` 省略 `parent_node`；>20MB prepare 显式 `parent_node=""`；`import_tasks` 始终带 `point.mount_type=1`；官方扩展名/大小矩阵（含 slides/base）；拒绝 wiki `--folder-token`。
- **Drive export**：docx markdown 走 `POST /docs_ai/v1/documents/{token}/fetch`（缺少 `data.document.content` fail-closed）；补齐类型/格式矩阵（slides/pptx、bitable/base、wiki 解析）。markdown 落盘走 output-dir 内路径校验 + 同目录 temp/fsync/rename，失败不截断已有 `--overwrite` 目标。
- **Drive move**：省略 `--folder-token` 时先 `GET /drive/explorer/v2/root_folder/meta` 取真实根目录 token；`task_check` 的 `task_id` 走 `url.Values` 编码。
- Markdown/import 分片：`block_size` 在转 int / 分配前做 max-int 校验；分片数用除法计算，避免 `(size+blockSize-1)` 溢出。
- `markdown diff`：每侧 10MB Stat + LimitReader 预检；`--format`/`--jq` 在下载前解析。
- 保留 Drive 大文件 hash / 远端重复路径 fail-closed / 重试词边界分类（0073317）。

### 新增 — 官方 OpenAPI catalog overlay（可回退、可缓存、无凭证）

- 编译期 `meta_data.json` 永远是离线 baseline；运行时可从官方 public `api_definition?protocol=meta` 拉 overlay。
- 5s 超时、10MB 硬限制、24h TTL；cache 原子写入（`~/.feishu-cli/cache/remote_meta.json` + metadata）；损坏 cache fail-closed 回退 embedded。
- 有 embedded baseline 时首次/TTL 到期走后台刷新，不阻塞启动；仅无 embedded 或品牌切换才同步拉取。
- overlay 要求 metadata 可解析、品牌匹配、version 非空且与 JSON `version` 完全一致、并新于 embedded；残缺 pair 不信任。
- 品牌切换删除旧 cache 文件并以 embedded 为 baseline 拉取；unchanged/失败不得把旧品牌数据改标后继续信任。
- `FEISHU_CLI_META_URL` 仅允许 loopback；正式请求/重定向仅当前品牌官方 HTTPS host，拒绝跨 origin 与 HTTPS→HTTP。profile root 失败时禁用 cache，不回退共享 `/tmp`。
- 远端 4xx/超时/超限/坏 JSON 不得让正常命令失败。请求不携带 App/User 凭证。
- 明确 opt-out：`FEISHU_CLI_REMOTE_META=off`；测试注入：`FEISHU_CLI_META_URL`（loopback）/ `FEISHU_CLI_CONFIG_DIR`。
- `schema status`、`doctor --only catalog`、`auth status` 报告 source（embedded/cache/runtime）、版本、service/method 数。
- scope 收集递归 nested resources。

### 新增 — 通用 `api --page-all/--page-limit`

- 仅识别 `data.has_more` + `page_token`/`next_page_token`；空/重复 cursor 停止并报错，不静默重复。
- `--page-limit` 截断时保留续翻 cursor，并输出 `truncated`/`page_count`；耗尽才删除 cursor。
- 多页必须有唯一可识别列表数组；未知字段仅在恰好一个数组时可用。初始 `page_token` 计入 seen。
- `--page-limit>=0`、`--page-delay>=0`、`--timeout>0` 在 token/网络前校验。
- 多页聚合保留 `json.Number` 大整数。业务 code != 0 非零退出。

### 修复 — `api` URL fragment / 官方 host

- 先剥 fragment 再解析 query，fragment 绝不能进入 query。
- 完整 URL 必须 https，且只接受 `open.feishu.cn` / `open.larksuite.com` / `open.larkoffice.com`；短 path 仍兼容。
- `--params` 用 `UseNumber` 解析，大整数 ID 不四舍五入。

### 修复 — 认证 / Token / SDK 传输安全

- SDK client 用 SHA-256 指纹检测 App Secret 变化：同长度轮换也会重建 client，进程内不保存 secret 明文。
- `auth token --as bot` 改为官方 Accounts OAuth v3 `POST /oauth/v3/token`（`grant_type=client_credentials`，form 编码），带超时、响应体大小限制、HTTP 与业务错误校验。官方 Open API 上 SDK 的旧 `tenant_access_token/internal` 换票由传输桥接到同一 v3 端点，业务请求不再把 App Secret 发到旧 Open host；`doctor` 的 bot_identity 复用 v3 fetcher。
- `auth token --as auto` 仅在「自然未配置 User Token」时回退 Bot；App mismatch / 文件损坏 / 刷新失败 fail closed。
- 显式 `--user-access-token` 与 `--as bot` 同时出现时直接报错，不再静默忽略其中一方。
- `token.json` 改为 0600 临时文件 + fsync + rename 原子写入；刷新在跨进程文件锁下执行 reload → check → refresh → commit，写失败保留旧文件。
- `token.json` 增加 `app_id` 绑定：与当前选中 App 不一致，或旧文件未绑定，一律 fail closed（即使 access 仍有效）。显式迁移：`feishu-cli auth token --bind-legacy-app --as user`（不可与 `--as bot` / `--user-access-token` 同时使用）或重新 `auth login`。
- `base_url` 默认只允许官方 HTTPS（`open.feishu.cn` / `open.larksuite.com`）；loopback HTTP 仅用于本机开发/测试。自定义远端 host、非 loopback HTTP、HTTPS→HTTP 或带 body 的跨源重定向必须分别设置 `FEISHU_ALLOW_CUSTOM_BASE_URL` / `FEISHU_ALLOW_INSECURE_HTTP` / `FEISHU_ALLOW_CROSS_ORIGIN_REDIRECT`（或对应配置项）。跨源重定向会剥离 `Authorization`，避免 App Secret 被外送。

### 修复 — 审批命令对齐官方当前 v4 用户态契约

全部 Approval 专用命令不再走旧 `uat_*` path，也不再把 `instance create` / `approval get` 当作 Tenant 应用态能力：

- `approval get` → `GET /open-apis/approval/v4/approvals/{approval_code}/detail`（User Token，`approval:approval:read`）
- `approval instance get` → `GET .../instances/detail`
- `approval instance initiated`（新增）→ `GET .../instances/initiated`
- `approval instance create` → `POST .../instances/initiate`（发起人取当前 User Token，不再传 `--user-id`）
- `approval instance cancel` → `POST .../instances/recall`
- `approval instance cc` → `POST .../instances/add_cc`
- `approval task query` → `GET .../tasks`（删除不存在的 `user_id` query；`count` 为整数；任务含 `instance_code` / `instance_status` / `initiator` / `initiator_name` / `summaries` / `support_api_operate`）
- `approval task approve/reject/transfer` → `POST .../tasks/pass|refuse|forward`

`--output raw-json` 在 HTTP 200 且飞书业务 `code != 0` 时非零退出，不再把失败 envelope 当成功输出。定义搜索、加签、退回、催办仍不在本命令面。

### 修复 — Calendar / IM 对齐官方当前契约

- `calendar event-search` 迁移到 `POST /calendars/{id}/events/search_event`，时间过滤写入 `filter.time_range`；`--calendar-id` 默认 primary、`--query` 可空；RFC3339/YYYY-MM-DD 单边补同一天边界；`page-size` 1–30 越界报错；`--as bot|user|auto` fail-closed；JSON 保留 `events`/`next_page_token` 并输出精确 `has_more`。
- `calendar agenda` 正确处理小于 40 天窗口、超 40 天预切分、193104 再切分去重；去掉 instance_view 伪分页；全天结束日按排他日期转为含当日；结束时刻用次日当地午夜减 1 秒（DST 安全）；`--as bot|user|auto` fail-closed。
- `search messages` 迁移到 `POST /im/v1/messages/search`；`--as bot|user|auto`（auto fail-closed）；query 可省略；补 `exclude_from_types` / `is_at_me` / `link`；`page-size` 1–50 越界报错；`--page-all` 最多 40 页、负数 `--page-limit` 拒绝；非法 `--format`/`--jq` 在身份解析前失败。
- `msg search-chats` 迁移到 `POST /im/v2/chats/search`，解析 `next_page_token`；`--as bot|user|auto`；`page-size` 1–100 越界报错；`--page-all` 最多 40 页。
- `msg mget` / `--enrich` 改用 `GET /im/v1/messages/mget`，每批最多 50，禁止 N+1。`msg history` 线程展开与 `with_sender_name` 保持不变。

### 变更 — `apps html-publish` 迁移到官方三段协议

退役单次 multipart POST `/apps/{id}/upload_and_release_html_code`。现按官方协议：

1. GET `/apps/{id}` 校验 `app_type` 为 `html` / `modern_html`
2. GET `/apps/{id}/pre_release` 解析 `upload_url` / `tos_path`
3. 对预签名 URL PUT tar.gz（**不携带飞书 Authorization**）
4. POST `/apps/{id}/releases`，body `{"tos_path":...}`，白名单返回 `release_id`（jq `.release_id`，不再返回 `.url`）

`--dry-run` 只展示三段计划与打包清单，不获取 token、不访问网络。HTTP/业务错误非零退出并带恢复建议；任一步失败中止后续调用。敏感文件扫描、index.html、尺寸上限保持不变。

### 修复 — Docs 与 Wiki 数据安全与 API 语义对齐

- **文档更新原子安全协议（PUT /open-apis/docs_ai/v1/documents/{id}）**：
  - `doc content-update` 全面迁移至官方单操作原子更新协议，`overwrite` 采用原子 overwrite，`replace_range` 采用原子 `block_replace`，`delete_range` 采用原子 `block_delete`，`replace_all` 倒序逐个原子替换且部分失败时非零并报告已完成项，彻底杜绝先删后写的数据破坏窗口。
  - 新增 `--revision-id` flag，透传版本号进行服务端乐观锁并发冲突保护。
  - 标题选择器：无 `#` 前缀支持匹配任意级别标题并精准截断章节范围，自动映射为实际 `start_block_id` 与 `end_block_id`。
  - 本地资源安全：检测到本地相对路径图片/文件时 fail closed 拒绝执行并提供迁移指引，确保远程原子操作真实保真。
- **文档子块删除分页健全**：
  - `doc delete --all` 改用 `GetAllBlockChildren` 全分页拉取父块下全部子块，彻底避免仅拉取第一页导致的漏删和谎报全删。
- **Wiki 节点删除与任务安全**：
  - 迁移至官方 `DELETE /open-apis/wiki/v2/spaces/{space_id}/nodes/{node_token}` 端点，请求体正确携带 `obj_type` 与 `include_children`。
  - 校验 `obj_type` 白名单（wiki, doc, docx, sheet, bitable, mindnote, slides, file），并对 space/node/task path segment 进行 URL 转义。
  - 异步任务轮询健全化：当轮询失败或超时/仍在 processing 时返回非零错误码，绝不谎报成功，并在错误信息中保留 `task_id` 与继续查询命令。
- **Wiki 快捷方式创建**：
  - `wiki create` 在 `--node-type=shortcut` 时强制校验并下发 `--origin-node-token`。
- **Wiki 节点检视**：
  - `drive inspect` 在展开 Wiki 节点时移除错误的 `obj_type=wiki` 查询参数。

## [v1.36.0] - 2026-07-22

本版为一次全域能力补齐：消息读取发送者名字服务端回填、CLI 交互健壮性守卫、OKR 全量接线、多维表格结构化过滤 DSL、电子表格类型保真读取闭环、大文档选择性读取、卡片交互回调与审批 v4 事件订阅，以及邮件/会议/纪要/云盘/任务/日历多域新命令。

### 新增 — 消息发送者名字服务端回填（with_sender_name）

- 所有读消息路径（`msg history/get/mget/thread-messages`、merge_forward 展开、线程展开）统一带 `with_sender_name=true`，服务端直接回填发送者显示名——**Bot 与外部租户用户也能解析**（此前 Bot 恒为空、外部用户约 42% 覆盖，实测内部群解析率 ~100%）。
- `ResolveSenderNames` 升级三步解析：服务端回填（权威）→ mentions 免费映射 → contact basic_batch 兜底；进程级注册表旁路采集（`internal/client/sender_names.go`）。

### 新增 — CLI 交互健壮性守卫

- **未知子命令不再静默成功**：嵌套命令组收到错拼子命令时返回错误 + 拼写建议 + 非 0 退出码（此前打印帮助并 exit 0，Agent 会误判执行成功）；裸命令组仍显示帮助。
- **flag 拼写建议**：未知 flag 报错时按编辑距离/前缀给出最相近候选（如 `--contaner-id` → `你是不是想用: --container-id`）。

### 新增 — OKR 全量接线 + `--as` 身份

- 新命令：`okr cycle detail`（周期下全部目标+关键结果）、`okr progress get/update/delete`、`okr upload-image`。
- 命令组新增 `--as bot|user|auto`（默认 bot 保持既有行为）；实测身份墙按端点分化：`cycle list` 仅收 Tenant Token，其余端点同时支持 user/tenant。

### 新增 — 多维表格结构化过滤（filter tuple DSL，实测验证）

- `bitable record list` 新增 `--filter-json` / `--sort-json`：无需关键词的纯结构化筛选（GET 端点 query 参数下发）。
- filter DSL 语法完整文档化（operator 全集 + 各字段类型 value 写法），7 种形态真实 API 验证通过；`record search` 的 filter 与 keyword 做交集的语义同步澄清。
- `record batch-update` 文档化两种 body 形态：统一 patch 与逐记录差异化（`update_records` map，实测均可用）。

### 新增 — 电子表格类型保真读取 `sheet table-put` 闭环

- 新命令 `sheet table-get`：按列类型保真读取整表（数字/日期/布尔自动推断 dtype，日期归一 ISO），输出与 `table-put` 输入完全对称，支持 get → 修改 → put 的 round-trip（已实测闭环）。

### 新增 — 大文档选择性读取 `doc read`

- `--outline` 标题大纲（层级 + block_id）、`--heading` 按标题取节（止于同级/更高级标题，代码围栏防误判）、`--keyword` 正则定位（`--context` 控制上下文），避免大文档整篇导出撑爆上下文。

### 新增 — 事件系统：卡片回调 + 审批 v4 订阅

- 新 EventKey `card.action.trigger`：卡片按钮/表单回调（独立回调帧通道），交互式 Bot 闭环补齐；`application.bot.menu_v6` Bot 菜单事件。
- 审批事件升级 v4 类型（`approval.instance/task.status_changed_v4`），consume 启动时自动以 User 身份注册服务端订阅关系（INVOLVED/MANAGED，此前旧 key 缺订阅注册收不到事件）。

### 新增 — 智能纪要入口 `vc note`

- `vc note detail <note_id>` 纪要详情、`vc note transcript <note_id>` 统一逐字稿导出（自动翻页，`--format markdown|text`，`--output` 落文件）。

### 新增 — 会议与妙记增强

- `vc detail <meeting_id|会议号>`：一条命令聚合会议基础信息 + `note_id`（智能纪要）+ `minute_token`（妙记），进行中会议不报错（部分产物缺失在 `hint` 标注）；会议号路径自动 90 天窗口反查。
- `minutes search`：按关键词/owner/时间搜索妙记；`minutes apply-permission --perm view|edit`：申请妙记权限；`minutes get --wait-ready`：轮询等待妙记转写就绪（`--wait-timeout`/`--wait-interval`）。

### 新增 — 群聊能力

- `chat list`：列出当前身份加入的全部群（User 身份实测 5677 群完整翻页），支持 `--sort-type`/`--page-all`。
- `chat member list --page-all`：自动翻页拉全量成员；命中服务端安全设置截断（返回条数 < member_total）时 stderr 中文告警，避免静默漏数据。

### 新增 — 邮件管理

- `mail message-modify`：批量加/删 label、移动文件夹（≤20 封，实测端点 `batch_modify`）；`mail message-trash`：批量软删进废纸篓（`--yes` 跳过确认，可用 message-modify 移回）；`mail draft-send`：发送已有草稿（沿用 `--confirm-send` 保护）。

### 新增 — 云盘与知识库

- `wiki move-to-drive`：把 wiki 节点移出知识库到云盘文件夹（异步轮询），补齐与 `wiki move-docs` 相反方向的闭环。
- `drive secure-label list/set`：密级标签查询与设置（User 身份）。
- `file version revert`：文件回滚到历史版本。
- `drive upload --file-token`：原地覆盖上传新版本（不改变权限设置）。
- `drive push` 命中 1062507（父目录子节点超 1500）按**父目录隔离**处理：已满目录及其子树内的条目标记失败跳过，其余目录继续镜像，收尾汇总列出已满目录与中文清理建议（此前对剩余文件盲目 continue 反复撞墙）。

### 新增 — 任务与日历

- `task search`：走服务端搜索端点，按创建者/执行者/关注者/完成态/截止时间/关键词过滤，支持 `--page-all`；已适配服务端硬限（page_size ≤30 自动收敛、翻页 offset 上限 150 优雅截断提示），5 并发补全任务详情。
- `calendar create-event/update-event --rrule`：RFC5545 重复日程（实测建→读回 recurrence 一致→改→删全链路）。

### 修复

- 未完成任务的 `completed_at` 哨兵值 `"0"` 此前被格式化成 1970 年时间，`task my` / `task get` / `task search` 现正确显示为未完成。

### 新增 — 平台工程化

- `msg send --idempotency-key`：发消息幂等键（≤50 字符，实测同 key 两次返回同一 message_id）。
- `doctor` 新增 `user_identity` / `bot_identity` 身份就绪诊断项。
- `auth logout` 默认先吊销服务端 token 再清理本地文件（`--no-revoke` 跳过；吊销失败不阻断本地清理）。
- `install.sh` 支持 checksums.txt sha256 校验（缺失时告警但兼容旧 release）。

### 重构 — Claude Code Skills 领域化

- 将 29 个顶层 Skill 整合为 9 个领域 Skill，原有细粒度能力下沉到各领域的 `references/workflows/`，降低路由歧义和常驻上下文占用。
- 新增 `skills/manifest.yaml` 作为能力归属清单，并新增 `scripts/check_skills.py` 校验目录结构、工作流引用、Go 源码中的 Skill 路径和 CLI 命令覆盖；`make check-skills` 会先重新构建，当前 405 个可执行命令（含隐藏命令）均有唯一归属。
- 为 9 个领域 Skill 补齐 33 个工作流执行评测，并增加每领域 8 个正例 + 8 个近邻负例的触发评测；同时修正认证、审批、消息、云盘、文档导入和可视化等工作流中与实际 CLI 行为不一致的说明。
- 同步更新 README、CLAUDE.md（及其软链接 AGENTS.md）中的安装清单、能力映射、迁移说明和脚本路径。

## [v1.35.0] - 2026-07-12

本版新增统一可视化设计系统、HTMLBox 编排动画生成器和多维表格记录搜索便捷模式，并修复 Wiki 导出、评论回复身份和安装脚本的已知问题。

### 新增 — 统一可视化设计系统（`feishu-cli-dataviz`）

- 新增图表形式选择、明暗主题色板、反模式清单和多载体配色规范，统一 board、HTMLBox、card 和 doc import 的可视化输出。
- `validate_palette.js` 新增 categorical、circular、all-pairs、HTMLBox 深色画布和 ordinal 共 9 组定稿门禁；重复色、首尾区分度和色盲安全性均可自动检查。
- `check_docs.js` 扫描技能文档中的 3/4/6/8 位 CSS hex，未登记的非 canonical 色值会直接阻断发布。

### 新增 — HTMLBox Agent 编排动画生成器

- `animate_diagram.py` 将结构化 JSON 转为自包含 SVG 动画 HTML，支持自动播放、暂停、上一步、下一步和进度定位。
- 补齐 `prefers-reduced-motion`、安全字幕渲染、Bezier/store 几何裁切和短时间线回归测试。

### 新增 — 多维表格记录搜索便捷模式

- `bitable record search` 新增 `--keyword`、可重复 `--search-field`、`--filter-json`、`--sort-json`、`--view-id`、`--offset` 和 `--limit`。
- `--config` / `--config-file` 保留完整请求体逃生舱，与便捷参数严格互斥；字段名中的逗号不再被错误拆分。

### 修复 — Wiki 树导出媒体路径

- `wiki export-tree --download-images` 为每篇文档使用独立素材目录，并将媒体引用改写为相对 Markdown 文件的路径。
- 覆盖 Quote、Callout、表格单元格内的多图片和 video，同时跳过 fenced/inline code、普通链接和未下载素材。
- 补齐 Windows 路径、空格、绝对/相对路径和转义反引号边界。

### 修复 — 评论删除与回复身份

- `comment delete` 不再伪装成可用的整条评论删除接口，改为返回删除自己回复或将评论标记为已解决的可执行指引。
- `comment reply add/delete` 默认统一使用当前 App/Bot 身份；只有显式提供 User Token 时才切换为用户身份，避免默认添加后无法默认删除。

### 修复 — 安装脚本版本获取

- `install.sh` 将过程日志统一写入 stderr，防止 GitHub API fallback 日志污染版本号和下载 URL。
- 增加版本号格式校验，异常响应会在发起下载前直接报错。

### 验证

- `go test -count=1 ./...`、`go vet ./...`、`gofmt`、Dataviz 九组色板门禁和 HTMLBox 5 个 Python 回归测试全部通过。
- 最终二进制实跑 Bitable/Comment 本地 mock 7 个场景全部成功；真实只读 Wiki 导出生成 3 个 Markdown 和 1 个素材，本地媒体引用 0 损坏。

## [v1.34.0] - 2026-06-28

新增 5 项能力 + 1 项修复，全部经真实飞书 API round-trip 验证。

### 新增 — 电子表格按列类型保真写入（`sheet table-put`）

把 pandas DataFrame 形状的 JSON（`to_json(orient="split")`）按列 dtype 写入电子表格，让数字/日期/文本列不被误判类型。

- 日期列写 Excel 序列号 + 给该列设日期 formatter（`yyyy/MM/dd`），飞书识别为「真日期」（可排序/可透视/ISNUMBER=TRUE），而非文本
- 数字列保数值精度（大整数不退化为科学计数法）；文本列用 `@` formatter 防止 ID/邮编等数字串被识别为数字（前导零保真，如 `007`）
- dtype 映射：`int*/uint*/float*/complex*`→number（`interval*` 除外，按文本）、`bool/boolean`→bool、`datetime*`→date、其他→string
- 空值（null/NaN）写空文本元素；单批 ≤ 5000 单元格自动按行分批
- ⚠️ 当前为写侧实现：**就地覆盖 A1 起的矩形区域、不清除区域外旧行、不自动扩容**（写入前用 `sheet add-rows` 预扩容）；仅支持单 sheet；读侧 round-trip（`table-get`）与 auto-grow 待后续

### 新增 — 画板服务端 SVG 导出（`board svg-export`）

`POST /board/v1/whiteboards/{id}/export`（export_type=svg），由服务端整板渲染为 SVG（base64 解码）。

- 与 `board export-code` 的区别：export-code 仅拼接 svg 节点的 svg_code，对 mermaid/plantuml/原生节点无效；svg-export 对任意画板有效，产出可二次编辑的完整 SVG
- 配合 `board import` / `svg_to_board.py` 实现「导出 → 编辑 → 回写」闭环
- 读类命令，登录后自动用 User Token，未登录回落 App Token

### 新增 — 审批实例指定节点审批人/抄送人（`approval instance create`）

- `--node-approver` / `--node-approver-file`、`--node-cc` / `--node-cc-file`：发起审批时按节点指定审批人/抄送人，格式 `[{"node_id":"n1","value":["ou_xxx"]}]`
- 新增 `skills/feishu-cli-approval/references/form-control-values.md`：14 类表单控件 value 结构速查 + 不支持清单 + 取值来源

### 新增 — `schema` pretty 输出渲染枚举值

`schema <service>.<resource>.<method>` 的 pretty 模式现在渲染字段的枚举取值（来自飞书归一化端点的 `options`/`enum`，含枚举描述），此前白白丢弃。数字型枚举值也正确渲染。

### 新增 — `apps html-publish` 单 .html 文件 10MB 上限

对齐妙搭服务端「单个 `.html` 文件 ≤ 10MB」硬约束，客户端提前拦截并点名超限文件。

- 实跑超限直接拒绝；`--dry-run` 回填 `oversize_html` 详情，并新增统一的 `would_block` / `block_reasons` 字段，便于脚本/Agent 单字段判断是否可发布

### 修复 — 电子表格图片上传适配 office 导入表格

`UploadSheetImageMedia` 的 `parent_type` 此前固定 `sheet_image`，对从 `.xlsx` 等导入的 office 表格（token 以 `fake_office_` 开头）会上传失败。现按 token 前缀自动选择 `office_sheet_file` / `sheet_image`。

## [v1.33.0] - 2026-06-27

### 新增 — 妙搭（Miaoda）应用：HTML 秒搭一键部署（`apps`）

新增 `apps` 命令组，把妙搭（Miaoda）低代码应用平台的「一份 HTML 秒级发布成可分享的飞书应用」能力搬进 feishu-cli。全部走 User 身份（user_access_token），需要 spark scope。

- `apps create` —— 创建 HTML 妙搭应用（`POST /open-apis/spark/v1/apps`），返回 `app.app_id`（CLI 已剥掉飞书响应的 `data` 外层，jq 用 `.app.app_id`）
- `apps html-publish` —— 把 `--path`（单 HTML 文件或整目录）打包成 tar.gz，单次 multipart POST 上传并发布（`/apps/{id}/upload_and_release_html_code`），返回 `url`（jq 用 `.url`，一键部署）。客户端侧：要求根目录有 `index.html`、未压缩 ≤ 200MB / 打包后 tar.gz ≤ 20MB、默认拦截凭证文件（`.env` / `.npmrc` / `.netrc` / `.git-credentials` / `.aws/credentials` / `.docker/config.json` / `.kube/config`，`--allow-sensitive` 放行）
- `apps update` —— 部分更新名称/描述（`PATCH /apps/{id}`）
- `apps access-scope-get` / `apps access-scope-set` —— 查看/设置访问范围（`specific` / `public` / `tenant`，映射后端 `Range` / `All` / `Tenant`）
- `apps list` —— 列出当前用户的应用（隐藏命令，游标分页）
- 统一支持 `--format json|pretty|table|ndjson|csv` + `--jq`；写命令支持 `--dry-run`
- 权限：`spark:app:write`（create/update/html-publish/access-scope-set）、`spark:app:read`（list/access-scope-get）。⚠️ feishu-cli 的 `auth login --scope` 是「替换」不是「合并」，请把 spark scope 并入完整 scope 串一起登录，避免丢失已有权限

### 改进 — Markdown 表格单元格图片真嵌入（#164）

Markdown 表格单元格内的本地/网络图片此前在转换阶段被静默丢弃（带 alt 只剩 alt 文本、无 alt 整格变空）。现在 `doc import` 会在表格填充完成后（阶段 2.5）真正嵌入为单元格内的 Image 子块。

- 新增 `ConvertOptions.EmbedTableImages` 开关，仅 `doc import` 启用真嵌入；非导入场景（`doc add/content-update`）单元格图片降级为 `[图片: 说明]` 占位文本，杜绝任何路径的静默丢失。
- 导入 JSON 输出新增 `cell_image_total/success/failed` 统计字段。

### 修复 — 发版前 code review 收尾

- **表格单元格图片（#164）**：纯图片单元格若带 alt（如 `![架构图](./a.png)`），alt 文本不再经填充兜底路径泄漏成单元格里多余的标题；嵌入阶段改用 `GetTableCellIDs`（`block.Table.Cells`，与填充/导出同源）定位单元格，不再依赖 list-blocks 是否为表格块填充 `children`；单元格数与图片索引不一致或获取失败时，被跳过的图片计入失败统计而非静默丢弃；单元格图片上传失败时删除孤儿空 Image 块并补占位文本
- **`apps` dry-run**：`--dry-run` 预览现在同样尊重 `--format/--jq`（此前固定 JSON，与 help 列出的 flag 不符）
- **`apps html-publish` 凭证扫描**：目录形态下不再因根的「父目录」恰好叫 `.aws/.docker/.kube` 而把根下普通 `credentials`/`config` 文件误判为凭证
- **文档**：`apps` 输出 jq 路径勘误（`data.app.app_id` → `.app.app_id`、`data.url` → `.url`，CLI 已剥掉 `data` 外层）
- **一致性**：内联图片占位统一为中文「[图片: …]」前缀（此前本地路径分支用英文 `[Image:` 且直出原始路径）；`apps html-publish` 打包错误信息改为中文

## [v1.32.0] - 2026-06-06

### 新功能 — 多维表格支持 `--as bot|user|auto` 身份切换

`bitable` 命令组此前在 CLI 侧**硬性强制 User Token**（未登录直接报错），但底层飞书 `base/v3` 与 `bitable/v1` API 本身一直**同时支持 User / Tenant(App) 两种身份**（client 早已声明 `SupportedAccessTokenTypes:[User, Tenant]`，瓶颈纯在命令封装层）。本版按 `--as bot|user|auto` 身份模式放开：

- 新增命令组 persistent flag **`--as bot|user|auto`**（默认 `auto`），所有 bitable 子命令通用：
  - `auto`（默认）：User 优先、Tenant 兜底——已登录用 User Token，**未登录/过期自动回落 App Token**
  - `bot`（= `tenant`/`app`）：强制 App Token，**无需 `auth login`、永不过期，适合 cron / 无人值守 / 脚本自动抓取多维表格**
  - `user`：强制 User Token（缺失报错，提示可改用 `--as bot`）
- 新增 `resolveIdentityToken`（`cmd/utils.go`）统一身份解析，替换 9 处咽喉点的 `resolveRequiredUserToken`/`requireUserToken`（`bitable_base`/`field`/`form_crud`/`output`/`record_attachment`）。客户端层零改动
- 修复后实测：在未登录环境用 App Token 读到此前因 token 过期而判定"不可入 cron"的真实多维表格全部 102 条记录

### 测试与文档

- 新增 `cmd/bitable_identity_test.go`：覆盖 `bot`/`tenant`/`app` 三别名（含大小写/空白）、显式 user token、非法 `--as` 报错、persistent flag 注册与子命令继承
- `feishu-cli-bitable` 技能 SKILL.md 补「身份选择 `--as`」表格 + cron 示例 + `91403` 协作者排错；description 补 cron / App Token 触发词
- `CLAUDE.md` Token 策略由「三类」扩为「四类」，新增 `resolveIdentityToken` 身份可选类目

## [v1.31.0] - 2026-06-05

### 新功能 — 妙笔BOX（htmlbox）HTML 小组件命令

飞书文档里**唯一能跑动画、可交互内容**的载体落地为正式命令。妙笔BOX 是 AddOns HTML 小组件块（`block_type=40`），把一整页 HTML 存进 `add_ons.record`，飞书在 iframe 沙箱里真实执行 CSS/JS——CSS 动画、ECharts、Three.js、Canvas、真实地图、3D 图表都能动（区别于画板的 SVG 节点会被服务端栅格化成静态图）。

**新增命令** `feishu-cli doc htmlbox {create|update|get|delete}`：

- `create`：往文档插入妙笔BOX 块（`--html` / `--html-file` / stdin 三选一，`--index` / `--parent-id` 控制位置）
- `update`：更新块 HTML。飞书 API 不支持原地改 `add_ons`（PATCH 返回 `1770001`），改走「**先建后删**、同位置重建」——新块在原位置创建成功后才删旧块，中途失败不丢数据，返回 `new_block_id`
- `get`：读回块 HTML（`--raw` 逐字节输出便于存文件/再编辑，默认输出含 `html` 字段的结构）
- `delete`：删除妙笔BOX 块（仅限 `block_type=40`，防误删其他块）
- 统一接入 `--format` / `--jq` / `--dry-run`；默认 Bot 身份（操作自建文档无需登录）

**配套 `feishu-cli-htmlbox` 技能**：SKILL.md + 3 个 references——`mechanism.md`（块机制 / iframe 沙箱边界 / 与画板的 trade-off）、`html-recipes.md`（CSS 动画 / ECharts / Canvas / Dashboard 自包含范例）、`pitfalls.md`（真实创建大批量图沉淀的 9 类实战踩坑：JS 报错白屏不报错、CDN 加载时序、真实地图 `registerMap`、`record` 双重编码、批量追加限流等）。

### 测试与质量

- 新增 7 个单测：record JSON 编码转义、`loadHTMLInput`（含「不 TrimSpace 保原文」这一 `get --raw` 还原保证）、`<script>` / HTML 注释 / `U+2028` payload roundtrip、unicode/emoji roundtrip
- 指针解引用统一用 `client.StringVal`，`update` 补 `len==0` 防御，`get --raw` 空内容时 stderr 告警

## [v1.30.0] - 2026-06-04

### 性能与功能 — 表格批量填充提速 25-30x，列宽可自定义

**① 表格填充 batch_update 优化（issue #159）**

`doc import` / `doc add` / `doc content-update` 三个入口的 Markdown 表格填充重写：

- 阶段二开始预热文档级 cellID → textBlockID 映射（一次 `GetAllBlocks` 替代 N 次 `GetBlockChildren`）
- single-group cell（占绝大多数）走 `batch_update` API，每批 ≤30 个一次写入；多块 cell（含 `<br/>`）保留原 update-first-empty 路径作为兜底
- 整批失败自动降级 per-cell，避免一颗坏 cell 污染整张表
- 新增文档级 3 QPS 写限流器（`docWriteLimiter`），下沉到 `CreateBlock`/`UpdateBlock`/`DeleteBlocks`/`BatchUpdateBlocks` 4 个底层写函数，所有间接调用者（`InsertTableRow`/`AppendTableRows`/`ReplaceImage` 等）自动受限，避免触发 99991400

**典型场景**：4 张 6×8 表（共 ~120 cells）从 ~70s 降到 ~3s（25-30x），与 issue #159 实测数据吻合。`BatchUpdateBlocks` 改为返回 `(*BatchUpdateBlocksResult, http.Header, error)`，让 retry 层拿到 `x-ogw-ratelimit-reset` 做精确退避。

**② 表格列宽自定义（issue #156）**

之前列宽完全由内容启发式计算（中文 14px / 英文 8px），用户无法干预。现在两种方式可覆盖：

- **紧邻表格上方注释**（推荐，单表精控）：

  ```markdown
  <!-- feishu-colwidth: 80,200,120,* -->
  | 列1 | 列2 | 列3 | 列4 |
  |-----|-----|-----|-----|
  ```

  单位支持 `px` 整数、`30%` 百分比（按 700px 文档宽度换算）、`*` 或空（该列走 auto）。注释独占一行才生效；中间夹任何块（heading/段落/列表/代码块/link-ref-def）会清空注释，避免悬浮注释污染下游表格。

- **CLI flag 全局覆盖**：`--table-column-width=auto|fixed|N1,N2,...`
  - `auto`（默认）：保留启发式
  - `fixed`：所有列等分文档宽度
  - 像素列表（如 `80,200,*,120`）：显式声明每列宽度，`*` 表示该列走 auto

**优先级**：注释 > CLI flag explicit > flag fixed > auto。所有路径最终都会过 `[80, 400]` 像素 clamp。注释/flag 列宽数量与表实际列数不一致时，stderr 打印警告（多写截断、少写补 auto）。

### 新增 — 消息搜索 enrich / 多维表格补全 / 输出工程化（jq + 表格/CSV）

补齐三类此前未覆盖的场景。

**① 消息搜索 enrich（`search messages`）**

新增 `--enrich` opt-in 富化：在消息 ID 基础上补全 **内容 / 发送者 / 群名 / 时间**（对齐 lark `+messages-search`）。默认行为保持原 ID 输出，完全向后兼容：

- 默认（无 `--enrich`）：仅返回消息 ID，`-o json` / `--format json` 返回旧 schema `{MessageIDs,HasMore,PageToken}`，与升级前一致
- `--enrich`：search → 消息 ID → `BatchGetMessages` 取详情 → 解析发送者名/群名 → 人类可读视图；`-o json` / `--format json` 返回 `[]{message_id,msg_type,chat_id,chat_name,sender_id,sender_name,create_time,time,text}` 数组
- `--format json|pretty|table|ndjson|csv` + `--jq` 结构化输出对两种模式均生效；`--page-all/--page-limit` 自动翻页

**② 多维表格补全（`bitable`）**

补齐 base/v3 + bitable/v1 公开 API 支持、此前缺失的命令：

- `bitable dashboard list|copy`（仪表盘）
- `bitable form get|patch` + `bitable form field list|patch`（表单及表单问题）
- `bitable role member list|create|delete|batch-create|batch-delete`（角色协作者）
- `bitable workflow enable|disable`（工作流启停）
- `bitable update`（多维表格本体重命名 / 高级权限开关）
- `bitable record batch-get`（批量获取记录）
- 新增 `internal/client/bitable_v1.go`：dashboard copy / role member / app update / workflow 启停 base/v3 无对应端点，走 bitable/v1（无需 `X-App-Id` header）。新命令经统一执行器 `bitableRun` + 请求描述符 `bitableReq` 路由 base/v3 与 bitable/v1，写命令支持 `--dry-run` 预览

> 注：dashboard block / form submit 已在后续一轮补齐（见下方「④ 多维表格 / 表格 / vc / mail / markdown 再补全」）。lark-base 的 view 独立 filter-sort 端点走飞书私有扩展 API（`base-api.feishu.cn`），公开 OpenAPI 无对应，未做专用命令——可用 `feishu-cli api` 裸调兜底。

**③ 输出工程化（`internal/output`）**

新增统一输出包，并接入 `api` 与新命令：

- `--format json|pretty|table|ndjson|csv`（`api` / `search messages` / bitable 新增命令）
- `--jq <expr>`（内置 gojq v0.12.17，纯 Go，无需外部 jq；保持 `go 1.21` 兼容）
- `api` 命令新增 `--format`/`--jq`（显式指定时走内置渲染，否则保持原 pretty/raw 行为）
- 大整数精度：渲染链路用 `json.Number`（飞书 19 位 message_id/chat_id 不被 float64 截断）
- table/csv 渲染 CJK 宽度对齐、单元格换行净化

**新增依赖**：`github.com/itchyny/gojq v0.12.17`（pin 该版本保持 `go 1.21` 兼容，未抬升 go directive）。

### 新增 — ④ 多维表格 / 表格 / vc / mail / markdown 再补全

补齐上一轮仍缺的仪表盘 / 表单 / 工作流 / 附件 / 浮图 / 筛选视图 / 下拉 / 会议机器人 / 邮箱签名 / Markdown diff。

**多维表格（`bitable`）**

- `bitable dashboard create|get|update|delete|arrange`（仪表盘 CRUD + 服务端智能排版，原有 `list|copy`）；`create|update` 支持便捷字段 `--name`/`--theme-style` 或 `--config`/`--config-file` 完整请求体二选一
- `bitable dashboard block create|get|list|update|delete`（仪表盘块 CRUD）；`create` 支持 `--type`（column/bar/line/pie/ring/area/combo/scatter/funnel/wordCloud/radar/statistics/text）+ `--data-config` 便捷字段
- `bitable form create|delete|detail|submit`（表单 CRUD + 按分享 token 取详情/提交，原有 `get|patch`）；`detail`/`submit` 走 `share-token`（shr 前缀）无需 base_token；`submit` 不处理附件上传
- `bitable form field create|delete`（表单问题批量增删，单次 ≤ 10，别名 `questions`，原有 `list|patch`）；`create` 用 `--questions` 数组，`delete` 用 `--question-ids`
- `bitable workflow create|get|update`（工作流 CRUD，`update` 为整体替换 PUT，原有 `enable|disable|list`）
- `bitable record upload-attachment|download-attachment|remove-attachment`（记录附件上传 / 下载 / 移除；upload/download 为 2 步编排，单次 ≤ 50 附件）

**表格（`sheet`）**

- `sheet image get|update|media-upload|write-image`（浮图获取 / 更新锚点尺寸偏移 / 上传素材取 file_token / 本地图片写入单元格，原有 `add|list|delete`）
- `sheet filter-view get|update`（筛选视图获取 / 更新名称范围，原有 `create|list|delete`）
- `sheet filter-view condition create|get|update|delete|list`（筛选条件 CRUD，按列字母定位）
- `sheet dropdown get|update|delete`（下拉菜单数据验证获取 / 更新 / 删除，原有 `set`）；`update` 支持多范围 / 多选 `--multiple` / 高亮 `--colors`
- `sheet batch-set-style`（批量为多范围设置单元格样式，`--data` 传 `{ranges,style}` 数组）

**会议（`vc`）**

- `vc bot meeting-join|meeting-leave|meeting-events`（会议机器人按会议号入会 / 离会 / 查会议事件，对应 `/open-apis/vc/v1/bots/{join,leave,events}`）

**邮件（`mail`）**

- `mail signature`（列出 / 查看邮箱签名，`--detail <签名ID>` 取单个详情）

**Markdown（`markdown`）**

- `markdown diff`（下载远端 Markdown 在本地算 unified diff，不改远端；三模式：远端最新 vs 本地 / 远端某版本 vs 最新 / 版本 A vs 版本 B）；输出支持 `--format`/`--jq`（`-o json` 作兼容别名），默认仍打印 unified diff 文本

**云盘下载（`drive` / `file` / `msg`）**

- `drive download` / `file download` 支持 User Token 直连下载 + 大文件自动 HTTP Range 分片兜底（突破单次下载大小限制；Bot/Tenant 路径仍保留 100MB 客户端上限）
- `msg resource-download` 支持 `--user-access-token` 用户身份下载消息图片/文件（可取 Bot 不可见的历史资源），大文件同样自动分片

## [v1.27.0] - 2026-05-21

### 新增 — `event` 模块（WebSocket 实时事件订阅 + daemon 进程管理）

新增 `feishu-cli event` 命令族（list / schema / consume / status / stop），
通过飞书 WebSocket 长连接接收应用事件并以 NDJSON 输出到 stdout。

**背景**：feishu-cli 之前完全没有事件订阅能力，AI Agent 想做 bot 实时响应只能自写 WebSocket
客户端。本 PR 在 feishu-cli 代码风格下重新实现，让单工具栈即可完成消息接收、群成员变更监听、审批
事件订阅等长连接场景。

**新增子命令**：

- `event list [--json]` — 列出所有支持的 EventKey（按 domain 分组：im / contact / calendar / drive / approval / vc 共 22+ 个）
- `event schema <key> [--json]` — 查看某个 EventKey 的 EventType / scope / payload schema 示例
- `event consume <key>` — 启动 WebSocket 长连接订阅（阻塞，事件流→stdout NDJSON）
- `event status [--json]` — 查看本机所有 consume 进程（PID/EventKey/启动时间/uptime）
- `event stop {--pid N | --event-key K | --all} [--force] [--json]` — 停止 consume 进程

**Consume 关键 flag**：

- `--max-events N` — 接收 N 条事件后退出（0=不限制）
- `--timeout 30s` — 运行时长上限（0=不限制）
- `--jq .event.message` — 极简点路径过滤（只接受 `.a.b.c`，不支持完整 jq 语法，用 pipe 接外部 jq）
- `--output-dir ./events` — 每条事件 dump 为 `<event_id>.json` 落盘（只接受安全相对路径）
- `--quiet` — 抑制 stderr 诊断（不影响 stdout 事件流；ready marker 仍会输出，便于父进程判断就绪）

**Daemon / 进程模型**：

- 每个 `event consume` 进程 = 一个独立 OS 进程 + 一个 WebSocket 长连接（一个 EventKey）
- 状态文件 `~/.feishu-cli/events/<app_id>/bus.json`（每个 AppID 一个目录）：consume 启动写入 PID/EventKey/启动时间，退出时移除
- 跨进程互斥 `~/.feishu-cli/events/<app_id>/bus.lock`（flock 文件锁，fd 关闭自动释放）
- 原子写：tmp + os.Rename 防止半写
- 进程探活：signal(0) 检测 PID 存活，status 命令自动清理僵尸条目
- 重连策略：复用 oapi-sdk-go v3 `ws.Client.WithAutoReconnect(true)`，断线无限重试（间隔 2 分钟 + 首次随机抖动）
- 架构取舍：不跑独立 daemon + Unix socket 做事件 fan-out；feishu-cli 简化为每个 consume 直连 WebSocket，
  不做事件分发——足够覆盖 AI Agent 单 EventKey 订阅的主线场景，省去 IPC 复杂度

**Subprocess 协议**（兼容 AI Agent 子进程调度）：

- 启动后 stderr 立即输出 `[event] ready event_key=<key>`，父进程应阻塞 stderr 等该行出现后再读 stdout
- 非 TTY 模式下 stdin EOF = shutdown 信号（适配 `< /dev/null` / `nohup` 等场景）
- 退出码 0：正常退出（达到 --max-events / --timeout / SIGTERM / Ctrl-C），非 0：startup 失败或 ws 不可恢复错误

**Scope 要求**：默认 App Token；具体 scope 因 EventKey 而异（`event schema <key>` 查看）。已加入
`--domain event --recommend` 推荐列表，覆盖 IM/联系人/日历/云盘/审批/VC 常用 scope 并集。

**代码影响范围**：

- 新增 `cmd/event.go`（顶层命令）+ `cmd/event_{list,schema,consume,status,stop}.go`（5 个子命令）
- 新增 `internal/event/{keys,bus,runtime}.go`（EventKey 注册表 + bus.json 状态管理 + WebSocket runtime）
- 新增 `cmd/event_test.go` + `internal/event/{keys,bus,runtime}_test.go`（mock 单测）
- 新增 `cmd/event_smoke_test.go`（`//go:build smoke` 本地真实 WebSocket 端到端测试）
- 修改 `internal/registry/domain_alias.go`：新增 `event` domain scope 推荐列表
- 修改 `go.sum`：补全 `larksuite/oapi-sdk-go/v3/ws` 子包的传递依赖（gorilla/websocket、gogo/protobuf；均为 indirect，无新顶层依赖）
### 新增 — `attendance` 考勤查询模块

新增 `attendance` 顶层命令组（别名 `att`），覆盖飞书考勤 OpenAPI 两类查询：

- `feishu-cli attendance user-task query` —— 按日期范围查询用户上下班打卡记录
  （`POST /open-apis/attendance/v1/user_tasks/query`，单次最多 50 用户）
- `feishu-cli attendance user-stats query` —— 查询日度 / 月度考勤统计
  （`POST /open-apis/attendance/v1/user_stats_datas/query`，单次最多 200 用户，
  起止跨度 ≤ 31 天）

**特性**：

- 日期参数同时接受 `YYYY-MM-DD` 与 `YYYYMMDD`，自动转换为 API 所需的 `yyyyMMdd` 整数
- 输出双模：默认 `text` 人类可读（打卡时间 / 结果 / 加班标记 / 统计字段标题），
  `-o json` 直出归一化结构体，便于 AI Agent 与脚本消费
- 同时打印 `invalid_user_ids` / `unauthorized_user_ids`，提示无效或无权限用户
- user-task ≤ 50、user-stats ≤ 200 用户数本地预校验，避免无谓远程请求
- user-stats 起止跨度 > 31 天本地预校验，避免触发 OpenAPI 报错
- 全部命令走 tenant_access_token（即应用身份）：larksuite/oapi-sdk-go v3.5.3 中
  `Attendance.UserTask.Query` / `Attendance.UserStatsData.Query` 的
  `SupportedAccessTokenTypes` 仅含 `Tenant`，传入 user token 会被 SDK 拒绝

**权限要求**：应用需在飞书开放平台「应用权限管理」页面获得
`attendance:task:readonly` 权限（tenant 级），无需 `auth login`。
### 新增 — `msg flag`：消息书签（收藏 / 列表 / 取消）

新增 `feishu-cli msg flag {create,list,cancel}` 三个子命令，对应飞书 OpenAPI `/im/v1/flags`，
覆盖消息书签的完整生命周期。

**支持的两层书签模型**：

| item_type  | flag_type | 场景                                |
| ---------- | --------- | ----------------------------------- |
| default    | message   | 消息层书签（最常见，默认值）        |
| thread     | feed      | topic-style 话题群 feed 层（侧边栏）|
| msg_thread | feed      | 普通群消息线程 feed 层              |

其余组合服务端会拒绝，CLI 默认值为 `default + message` 即可覆盖 90% 用例。

**实现说明**：飞书 Open SDK v3 当前未封装 flag 接口，使用通用 HTTP client（`client.Post` /
`client.Get`）直接调用，与 `comment reply add` 同套路。

**权限要求**：User Token；`list` 需要 `im:feed.flag:read`，`create/cancel` 需要 `im:feed.flag:write`
### 新增 — `okr` 模块：OKR 周期和进展记录

新增 `feishu-cli okr` 命令组，覆盖 OKR 最高频的 3 个操作：

- `okr cycle list` — 获取当前租户的所有 OKR 周期（`/open-apis/okr/v1/periods`，租户级全局列表，自动分页）
- `okr progress list --objective-id 7xxx | --key-result-id 7xxx` — 列出某个目标 / 关键结果下的所有进展记录
- `okr progress create --objective-id 7xxx | --key-result-id 7xxx --content "..."` — 创建一条进展记录，
  支持纯文本（`--content`，自动包装为 ContentBlock）或原始富文本（`--content-json`）；
  可附带 `--progress-percent` + `--progress-status` 标记进度；
  `--source-url` 飞书侧必填，CLI 默认填 `https://www.feishu.cn/okr/progress` placeholder，可显式覆盖

**实现要点**：

- `progress create` 走飞书 Open SDK v3.5.3 的 `Okr.ProgressRecord.Create`；
  `cycle list` 走通用 HTTP client 直调 `/open-apis/okr/v1/periods`（v1/periods 是租户级，不按用户过滤）；
  `progress list` 走通用 HTTP client 直调 `/open-apis/okr/v2/...`
- 所有命令默认使用 User Token，会自动读取 `~/.feishu-cli/token.json`；
  也可以通过 `--user-access-token` 或 `FEISHU_USER_ACCESS_TOKEN` 显式覆盖
- 时间戳统一格式化为本地时区 `YYYY-MM-DD HH:MM:SS`，方便人眼阅读

**权限要求（User Token scope）**：

| 命令              | scope                                              |
|-------------------|----------------------------------------------------|
| `cycle list`      | `okr:okr:readonly` 或 `okr:okr.period:readonly`    |
| `progress list`   | `okr:okr:readonly` 或 `okr:okr.progress:readonly`  |
| `progress create` | `okr:okr` 或 `okr:okr.progress:writeonly`          |

**使用示例**：

```bash
feishu-cli auth login --domain event --recommend

# 列出所有 EventKey
feishu-cli event list

# 查看 IM 接收消息事件的字段
feishu-cli event schema im.message.receive_v1

# 订阅（Ctrl-C 退出）
feishu-cli event consume im.message.receive_v1

# 调试：抓 5 条事件后自动退出
feishu-cli event consume im.message.receive_v1 --max-events 5 --timeout 60s

# 并发订阅多个 EventKey（每个进程一个 EventKey）
feishu-cli event consume im.message.receive_v1 > receive.ndjson 2> receive.log &
feishu-cli event consume im.chat.member.user.added_v1 > member.ndjson 2> member.log &
feishu-cli event status                       # 查看活跃进程
feishu-cli event stop --all                   # 一键停止
```

### 新增 — `doctor` 命令（健康检查 / 配置 / 认证 / 网络 / 依赖一把验）

新增 `feishu-cli doctor` 命令，跑一组本地诊断快速验证 CLI 状态。

**6 项检查**：
- `config_file` — app_id / app_secret 是否就位
- `user_token` — token.json 状态（valid / needs_refresh / expired）
- `endpoint_open` — `open.feishu.cn` HTTPS 可达性 + RTT
- `endpoint_larksuite` — `open.larksuite.com` HTTPS 可达性 + RTT
- `proxy` — HTTPS_PROXY 与 NO_PROXY 配置（缺飞书域 warn）
- `dependencies` — Go 版本 + larksuite/oapi-sdk-go 版本

**flag**：`--json` 机器可读输出 / `--offline` 跳过网络检查 / `--only user_token,proxy` 仅运行指定项。

**退出码**：0 = 全 pass / 1 = 任一 fail。

**使用示例**：

```bash
feishu-cli doctor                              # pretty 输出全检查
feishu-cli doctor --json                       # JSON 输出（AI agent 自检友好）
feishu-cli doctor --offline                    # 跳过网络
feishu-cli doctor --only user_token,proxy      # 仅跑指定项
```

**代码影响范围**：新增 `cmd/doctor.go`（6 项检查 + pretty/JSON 输出）和 `cmd/doctor_test.go`（parseOnly / shouldRun / proxy / dependencies 单测）；不引入新依赖。

### 新增 — `slides` 模块：Slides 演示文稿创建与媒体上传

新增 `feishu-cli slides` 顶层命令，提供两个子命令支撑 Slides 演示文稿的最小可用工作流：

- `slides create [--title <name>] [--width <px>] [--height <px>] [--output json]`
  调用 `POST /open-apis/slides_ai/v1/xml_presentations` 创建空白演示文稿，返回 `xml_presentation_id` /
  `revision_id` / `title`。默认尺寸 960x540。
- `slides media-upload --file <path> --presentation-token <xml_presentation_id> [--output json]`
  本地图片走 `/open-apis/drive/v1/medias/upload_all` 上传到指定演示文稿，返回 `file_token`
  可直接作为 slide XML 中 `<img src="...">` 引用。

**关键实现细节**：

- 上传 `parent_type` 固定为 `slide_file`（实测：`slide_image` / `slides_image` /
  `slides_file` 都会被拒）；`parent_node` 必须为目标 `xml_presentation_id`
- 单文件上限 20 MB（多分片 `upload_prepare` 不接受 `parent_type=slide_file`）
- 共用 `internal/client/drive.go::UploadMediaWithExtra` 上传链路

**权限要求**：

- 创建：`slides:presentation:create` 或 `slides:presentation:write_only`
- 上传：`docs:document.media:upload`
### 新增 — `mail` 高级能力：CID 内联图片 + 邮件模板（MVP）

为 `mail` 模块补齐两块进阶能力：直接发送与邮件模板。

**1. `mail send --inline-images-auto-scan`（CID 内联图片）**

HTML body 中所有 `<img src="本地相对/绝对路径">` 会被自动扫描：

1. 跳过已经是 `cid:` / `http(s):` / `data:` / `//` scheme 的引用
2. 同一本地路径只上传一次（去重）
3. 每张图独立生成 20-hex CID
4. 走 `drive/v1/medias/upload_all`（`parent_type=email`，`parent_node = 当前登录用户 open_id`）
5. EML 走 `multipart/related`：HTML 段 + 每张图一个 `Content-ID: <cid>`、`Content-Disposition: inline` 的 part
6. 改写 `src` 为 `cid:<cid>` 后回写到 body

依赖 `~/.feishu-cli/user_profile.json` 中缓存的 open_id（`auth login` 后自动写入）。

**2. `mail template create` / `mail template list`（邮件模板 MVP）**

- `mail template create --name xxx --subject xxx --body xxx [--to ... --cc ... --bcc ... --plain-text]`
  调用 `POST /open-apis/mail/v1/user_mailboxes/{id}/templates`
- `mail template list [--mailbox me]`
  调用 `GET /open-apis/mail/v1/user_mailboxes/{id}/templates`（接口不分页，一次返回所有 id+name）

底层 client 也实现了 `GetMailTemplate` / `UpdateMailTemplate` / `DeleteMailTemplate`，但 CLI 层目前只暴露 create/list（MVP）。

**权限要求**：

- User Access Token
- `mail:user_mailbox:readonly` / `mail:user_mailbox.message:modify` / `mail:user_mailbox.message:send`
- 模板相关 scope：`mail:user_mailbox:readonly`、`mail:user_mailbox.message:modify`

⚠️ **模板接口依赖邮箱读写相关权限** —— 命令本身实现完整、参数校验完整、EML/JSON payload
正确；如果调用模板 API 时返回 scope 校验失败（401/permission denied），请补开邮箱读写权限后重试。
CID 内联图片功能不依赖此 scope，已可正常使用。
### 新增 — `profile`：多配置（profile）管理

新增 `feishu-cli profile` 顶层命令，让一台机器在多个飞书账号 / 应用之间快速切换。
解决长期痛点：原 `~/.feishu-cli/{config.yaml,token.json}` 单实例布局，切账号必须手动备份/恢复
或者来回 `mv`，对同时需要 work / personal、或者 feishu.cn / larksuite.com 双端的用户极不友好。

**子命令**：

- `profile add <name> [--app-id ... --app-secret ... --base-url ... --use]` 新建 profile
- `profile list` (alias `ls`) 列出所有 profile，标注 active 列；`--json` 适合脚本/AI Agent
- `profile use <name>` (alias `switch`/`checkout`) 切换 active；`use -` 切回上一个
- `profile current` 显示当前 active profile 名 + 目录
- `profile rename <old> <new>` (alias `mv`) 重命名，自动同步指针
- `profile remove <name>` (alias `rm`/`delete`) 删除 profile；`--force` 跳过二次确认
- `profile migrate [--name default] [--force]` 把旧布局 `~/.feishu-cli/{config,token}.json` 拷到 `profiles/<name>/`（原文件保留，让用户确认无误后手动清理）

**目录布局**：

```
~/.feishu-cli/
  config.yaml                # 旧布局，profile 系统未启用时仍读这里（无感升级）
  token.json
  active-profile             # 一行文本：当前 profile 名
  previous-profile           # 一行文本：上一个 profile 名（支持 use -）
  profiles/
    work/
      config.yaml
      token.json
      user_profile.json
    personal/
      ...
```

**向后兼容设计**：

- 没有任何 profile 时，`internal/config` 和 `internal/auth` 仍走旧路径，老用户零感知升级
- `profile add` **不会** 自动迁移旧文件——避免静默丢数据；要迁就显式 `profile migrate`
- `FEISHU_PROFILE=<name>` 环境变量临时覆盖（不写指针文件），适合 CI / 一次性切换

**安全**：

- profile 名仅允许 `[A-Za-z0-9_-]{1,64}`，禁止 `.`/`..`/路径分隔符等注入字符
- 保留名 `profiles` / `cache` 不可作为 profile 名
- 写入操作通过进程内 mutex 串行化；指针文件原子写（`.tmp` + rename）
- 所有 profile 目录默认 `0700` 权限，含 token.json 等敏感文件

**测试**：`internal/profile/store_test.go` 21 个测试，全部用 `t.TempDir()` 隔离，覆盖
ValidateName / List 字典序 / Create / Remove / Rename / Use 含 `-` 切换 / `MigrateLegacy`
含 `--force` 覆盖 / `FEISHU_PROFILE` 环境变量优先级 / `ActiveDir` 新旧布局切换。

**示例**：

```bash
# 创建一个标题为 "Q2 OKR" 的演示文稿
feishu-cli slides create --title "Q2 OKR" --output json

# 把封面图上传到该演示文稿
feishu-cli slides media-upload --file ./cover.png \
    --presentation-token <xml_presentation_id>
```
### 新增 — `schema` 命令：本地浏览飞书 OpenAPI 方法（path / 参数 / scope）

新增 `feishu-cli schema [service.resource.method]` 子命令，无需 token、纯本地查询飞书
开放平台 OpenAPI 方法的 HTTP path / 动词 / 参数 / 请求体 / 响应体 / scope / 文档链接。
便于 AI Agent 和脚本作者快速查找参数。

**用法**：

```bash
feishu-cli schema                                # 列出所有可用 service（12 个）
feishu-cli schema im                             # 列出 im 域下所有 resource.method
feishu-cli schema im.messages                    # 列出 messages 资源下所有 method
feishu-cli schema im.messages.delete             # 查看具体 method 详情
feishu-cli schema im.messages.delete --format json   # JSON 输出（AI Agent 推荐）
feishu-cli schema list --service drive           # 等价于 schema drive，支持 --format json
```

**数据源**：`internal/registry/meta_data.json`（编译期 embed），与认证模块复用同一份元数据。
当前覆盖 12 个 service：approval / attendance / calendar / drive / im / mail / minutes /
sheets / slides / task / vc / wiki。

**输出含**：HTTP verb + 完整 path、parameters（含 path / query / required 标记）、
requestBody（嵌套字段）、responseBody、accessTokens（user / tenant）、scopes、docUrl。

# 查询本人最近一周打卡
feishu-cli attendance user-task query \
    --employee-type open_id \
    --user-ids ou_xxxxxxxxx \
    --start 2026-05-01 --end 2026-05-18

# 查询本月日度统计（JSON 输出）
feishu-cli attendance user-stats query \
    --employee-type open_id \
    --user-ids ou_xxxxxxxxx --current-user-id ou_xxxxxxxxx \
    --stats-type daily --start 2026-05-01 --end 2026-05-31 -o json
# 收藏消息（消息层）
feishu-cli msg flag create om_xxx

# feed 层书签（自动识别 thread/msg_thread）
feishu-cli msg flag create om_xxx --flag-type feed

# 列出当前用户所有书签
feishu-cli msg flag list --page-size 50

# 取消书签（默认尽量取消消息层 + feed 层）
feishu-cli msg flag cancel om_xxx

# feed 层书签（普通群线程）
feishu-cli msg flag create om_xxx --item-type msg_thread --flag-type feed
```
feishu-cli auth login --scope "okr:okr"
feishu-cli okr cycle list
feishu-cli okr progress list --objective-id 7xxx
feishu-cli okr progress create --key-result-id 7xxx --content "本周完成核心模块联调"
```

**MVP 范围说明**：本次只覆盖最常用的 3 个动词，progress update / delete / get 和图片上传暂不暴露
为命令行（client 层已有实现，后续按需补 CLI）。
### 新增 — `approval` 流程：实例详情 / 发起 / 撤回 / 抄送 / 通过 / 拒绝 / 转交

补齐审批模块的核心能力，原本只有 `approval get`（定义查询）和 `approval task query`（任务列表查询）两条只读命令，现在可以覆盖官方当前可执行的实例/任务主路径：

- `feishu-cli approval instance get` — 获取单个审批实例详情，对齐官方 `instances/uat_get`
- `feishu-cli approval instance create` — 发起一条审批实例，`--form` 或 `--form-file` 传表单 JSON
- `feishu-cli approval instance cancel` — 撤回（取消）已发起的审批实例
- `feishu-cli approval instance cc` — 把审批实例抄送给一个或多个用户（`--cc-user-ids ou_a,ou_b`）
- `feishu-cli approval task approve` — 通过指定审批任务，可附 `--comment`
- `feishu-cli approval task reject` — 拒绝指定审批任务，建议在 `--comment` 中填写原因
- `feishu-cli approval task transfer` — 转交审批任务给其他用户，对齐官方 `tasks/uat_transfer`

**权限要求**：`instance get`、`task query`、`instance cancel/cc`、`task approve/reject/transfer` 使用 User Token，分别需要 `approval:instance:read`、`approval:task:read`、`approval:instance:write`、`approval:task:write`；`instance create` 是本项目额外应用态能力，使用 tenant_access_token，需要 `approval:approval`。

**底层 API**：

- `GET /open-apis/approval/v4/instances/uat_get`
- `POST /open-apis/approval/v4/instances`
- `POST /open-apis/approval/v4/instances/uat_cancel`
- `POST /open-apis/approval/v4/instances/uat_cc`
- `POST /open-apis/approval/v4/tasks/uat_approval`
- `POST /open-apis/approval/v4/tasks/uat_reject`
- `POST /open-apis/approval/v4/tasks/uat_transfer`

官方 skill 文案中提到但当前官方可执行 schema 未开放的能力仍不在本次范围：`tasks/rollback`（退回）、`tasks/add_sign`（加签）、`tasks/remind`（催办）。

**代码影响范围**：

- `internal/client/approval.go`：新增审批实例详情、实例写、任务写/转交 client 函数 + 对应 Options 结构 + 共享请求 helper
- `cmd/approval_instance.go`：新增 `approval instance` 父命令
- `cmd/approval_instance_{get,create,cancel,cc}.go`：4 条实例侧子命令
- `cmd/approval_task_{approve,reject,transfer}.go`：3 条任务侧子命令
### 新增 — `sheet filter-view` + `sheet dropdown`：筛选视图与下拉菜单

补齐两块电子表格高级能力：

- **筛选视图 CRUD（V3 API）**：用 SDK `SpreadsheetSheetFilterView` 实现
  - `feishu-cli sheet filter-view create --token <t> --sheet-id <s> --range "<sheetId>!A1:H14" [--name 视图名 --filter-view-id 自定义ID]`
  - `feishu-cli sheet filter-view list --token <t> --sheet-id <s>`
  - `feishu-cli sheet filter-view delete --token <t> --sheet-id <s> --filter-view-id <fv>`
  - `--range` 不带 sheetId 前缀时自动补全为 `<sheet-id>!<range>`
- **下拉菜单（V2 dataValidation API）**：list 类型数据验证
  - `feishu-cli sheet dropdown set --token <t> --range "<sheetId>!A1:A100" --options "待办,处理中,已完成" [--multiple --colors "#FF4D4F,#FAAD14,#52C41A"]`
  - `--options-json '["a, b","c"]'`：选项内含逗号时绕过 CSV 解析
  - 传 `--colors` 自动开启 `highlightValidData`，颜色数量需与选项一致

**权限**：`sheets:spreadsheet`（User Token 或 App Token 均可），命令默认 `resolveOptionalUserTokenWithFallback` 自动读取登录态。

**代码影响范围**：

- `internal/client/sheets.go`：新增 `CreateFilterView` / `ListFilterViews` / `DeleteFilterView` / `SetDropdown`
- `cmd/sheet_filter_view.go`、`cmd/sheet_dropdown.go`：CLI 入口
### 新增 — `markdown {create,fetch,overwrite}`：Drive 原生 .md 文件 CRUD

新增 `feishu-cli markdown` 顶层命令，把 Drive 上的 `.md` 当作普通文件整体读写，
保留原始 Markdown 格式（**不做** Markdown ↔ 飞书 docx 块的转换）。

**与 `doc import` / `doc export` 的区别**：

| 命令 | 行为 | 创建出的文档类型 |
|------|------|------------------|
| `doc import/export` | Markdown ↔ 飞书 docx 块（标题/列表/表格/Callout…） | docx |
| `markdown create/...` | 把 `.md` 整体上传/下载，不做转换 | file（普通 Drive 文件） |

适合 AI agent 把生成的 Markdown 直接落盘到飞书 Drive、下次读回时仍是原汁原味
Markdown 源码的场景。

**子命令**：

- `markdown create --name xxx.md --content "..." | --content-file path.md | --file path.md [--folder-token fldxxx]`
  从字符串或本地文件创建 `.md`；强制 `.md` 后缀；空内容报错；底层走
  `client.UploadFileWithToken`（≤ 20MB 单次上传，> 20MB 复用现成分片管线）。

- `markdown fetch --file-token boxcnxxx [--output-path path] [-o json]`
  缺省 `--output-path` 时直接打印到 stdout；
  指定路径则落盘，目录会拼 `fileToken.md`，`--overwrite` 防误覆。

- `markdown overwrite --file-token boxcnxxx --name existing.md --content "..." | --content-file path.md | --file path.md [--name renamed.md]`
  覆盖现有 `.md` 的内容，`file_token` 保持不变；`--content` 时 `--name` 必填，`--content-file` 缺省使用本地 basename。
  **实现细节**：飞书 Go SDK v3.5.3 的 `UploadAllFileReqBody` 没有暴露 `file_token`
  字段，因此本命令用 `client.Post` + `*larkcore.Formdata` 自己拼 multipart，
  endpoint 仍是官方的 `POST /open-apis/drive/v1/files/upload_all`，
  `shortcuts/markdown/helpers.go` 的写法。

**权限**：User Access Token + `drive:file:upload` / `drive:file:download`
（或 `drive:drive`）。

# 内联图片
feishu-cli mail send --to user@example.com --subject "周报" \
    --body '<p>看附图</p><img src="./screenshot.png">' \
    --inline-images-auto-scan --confirm-send

# 模板创建+列表
feishu-cli mail template create --name "周报" --subject "本周进度" --body "<p>模板</p>"
feishu-cli mail template list
### 新增 — `calendar` 智能化三件套（suggestion / room-find / rsvp）

针对 AI Agent 自动排会场景，补齐三条飞书日历开放能力，使整条「选时段 → 选会议室 → 答复邀请」
流水线全部可在 CLI 完成。

- **`calendar suggestion`**：智能时段建议。直调 `POST /open-apis/calendar/v4/freebusy/suggestion`，
  按 `--attendee-ids ou_xxx,oc_yyy` + `--duration 30m/1h30m/90` 推荐可用时段；支持
  `--start`/`--end` 搜索窗口（默认当天）、`--exclude start~end,...` 排除午休/已占用时段、
  `--event-rrule` 周期性规则、`--timezone`。返回带「推荐理由」+「AI 行动指引」。
- **`calendar room-find`**：会议室查找。直调 `POST /open-apis/calendar/v4/freebusy/room_find`，
  支持多个 `--slot start~end` 并发查询（worker=10），可按 `--city`/`--building`/`--floor`/
  `--room-name`（逗号分隔多个）/`--min-capacity`/`--max-capacity` 多维度过滤；可选
  `--attendee-ids` 让服务端结合参与者位置筛选。
- **`calendar rsvp`**：答复日程邀请。走 SDK Reply 接口，`--calendar-id`（可省略，默认主日历）+
  `--event-id` + `--action accept|decline|tentative`。与既有的 `calendar event-reply`
  位置参数风格互为补充——rsvp 全 flag 风格、calendar-id 可省，更适合 AI Agent 调度。

**SDK 现状**：v3.5.3 暴露 `Reply` 但未暴露 `freebusy/suggestion` 和 `freebusy/room_find`，
故 suggestion / room-find 走 `client.Post` 通用 HTTP 直调 OpenAPI；新增 client 函数集中在
`internal/client/calendar_smart.go`，包括 `SuggestFreebusy`、`FindMeetingRoom`、
`FindMeetingRoomBatch`（并发+排序）、`SplitAttendeeIDs`（按 `ou_`/`oc_` 前缀分流）。

**权限要求**：
- suggestion / room-find：`calendar:calendar.free_busy:read`（User Token 或 App Token 均可）
- rsvp：`calendar:calendar.event:reply`（推荐 User Token，以本人身份答复）

**典型用法**：

```bash
# 1. 先让飞书推荐可用时段
feishu-cli calendar suggestion --attendee-ids ou_aaa,ou_bbb --duration 30m

# 2. 锁定时段后查会议室
feishu-cli calendar room-find \
  --slot 2024-01-22T09:00:00+08:00~2024-01-22T09:30:00+08:00 \
  --building "飞书大厦" --min-capacity 6

# 3. 收到邀请后答复
feishu-cli calendar rsvp --event-id EVENT_xxx --action accept
# 从旧布局开始（已有 config.yaml 和 token.json）
feishu-cli profile migrate                              # → profiles/default/，指针指 default
feishu-cli profile add personal --use --app-id cli_yyy  # 新建 personal 并切过去
feishu-cli profile list                                 # 看哪个 active
feishu-cli profile use -                                # 切回 default
FEISHU_PROFILE=personal feishu-cli msg send ...         # 一次性临时切换
```

### 新增 — `comment reply add`：为已有评论添加回复

新增命令 `feishu-cli comment reply add <file_token> <comment_id> --text "..."`，补齐评论回复
生命周期的最后一块拼图（此前只有 list / delete）。

**背景**：飞书 Open SDK v3.5.3 的 `fileCommentReply` 只暴露 `List`/`Delete`/`Update`，没有
`Create` 方法，而 Open API 本身是支持的（`POST /drive/v1/files/:token/comments/:comment_id/replies`）。
此 PR 不依赖 SDK 升级，用通用 HTTP client（`client.Post`）直接调用 API 实现。

**同时改进**：

- `comment reply add` / `delete` / `list` 全部加上 `--user-access-token` 参数支持，并走
  `resolveOptionalUserTokenWithFallback` 自动读取登录态，和 msg/chat/doc export 等模块保持一致
- **重要修复**：`comment reply delete` 在 App Token（Bot 身份）下调用飞书侧会返回 `1069303
  forbidden`——飞书只允许回复作者本人删除。现在命令默认优先使用 User Token（如果已登录），
  行为才符合用户预期。命令帮助中也显式说明了这个权限模型
- `comment reply add` 默认也走 User Token fallback，回复会以用户身份发布（而非显示为 Bot），
  且该回复能被后续 `reply delete` 正常删除

**权限要求**：`docs:document.comment:create`（User Token）

**使用示例**：

```bash
feishu-cli auth login                       # 确保有 User Token
feishu-cli comment reply add <file_token> <comment_id> --text "已处理"
feishu-cli comment reply delete <file_token> <comment_id> <reply_id>  # 自动用 User Token
```

**代码影响范围**：

- `internal/client/comment.go`：新增 `CreateCommentReply`（HTTP client 直调），
  `ListCommentReplies` / `DeleteCommentReply` 签名增加 `userAccessToken` 参数
- `cmd/comment_reply.go`：新增 `addReplyCmd`，三个子命令统一加 `--user-access-token` flag
- `cmd/comment.go`：Long help 中补充 reply add 示例

### Features — 新增 `wiki move-docs` 命令（移动云空间文档至知识空间）

新增 `feishu-cli wiki move-docs <obj_token> --space-id <id>` 命令，对应飞书 OpenAPI `POST /open-apis/wiki/v2/spaces/{space_id}/nodes/move_docs_to_wiki`。

**解决的问题**：之前要把"我的空间 / 共享空间"里已存在的 docx / sheet / mindnote / bitable / file 挂到知识库下，只有两条路——(1) 飞书客户端手动点"添加到知识库"；(2) 走 `wiki create` 新建空文档再重写内容。前者不能自动化，后者丢原文档权限和历史。新命令一步到位。

**用法**：

```bash
# 把 drive docx 移入知识空间根目录
feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567

# 移入指定父节点
feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --parent-node wikcnYYYYYY

# 移动电子表格
feishu-cli wiki move-docs shtcnXXXXXX --space-id 7012345678901234567 --obj-type sheet

# 无 move 权限时提交迁入申请
feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --apply

# 用用户身份调用（企业版 wiki 空间不接受 app 成员，必须 user token）
feishu-cli wiki move-docs doccnXXXXXX --space-id 7012345678901234567 --user-access-token u-xxx
```

**返回三种情况**：`wiki_token`（立即完成）/ `task_id`（异步任务）/ `applied=true`（权限不足已提交申请）。

**Scope 要求**：`wiki:node:move` 或 `wiki:wiki`，已加入 `--domain wiki --recommend` 推荐列表。

**代码影响范围**：
- 新增 `cmd/move_docs_to_wiki.go`（命令）和 `internal/client/wiki.go` 的 `MoveDocsToWiki` 函数
- `internal/registry/domain_alias.go` 的 `wiki` domain 补上 `wiki:node:move`
- README 知识库操作段落补一行命令

---

### Breaking Changes — 移除 `config add-scopes` 命令

`feishu-cli config add-scopes` 子命令及其 `--domain` / `--scopes` / `--print-only` flag 全部删除。

**删除理由**：

1. **命令几乎不可用** — 硬编码的 `scopeDomains` 字典里多数 scope 名已过时（`docx:document` / `sheets:spreadsheet` / `bitable:app` / `im:chat:readonly` / `drive:export:readonly` / `vc:room:readonly` 等都是飞书不支持的粗粒度名称），生成的申请链接里多数 scope 会被后台拒绝
2. **权限开通不适合自动化** — 飞书开放平台的权限申请通常需要 tenant 管理员审批，scope 选择也是业务决策而非技术"默认值"。CLI 自动化只会造成"看起来装好了但后台还没批"的幻觉
3. **有更简单的替代** — 飞书开放平台的应用权限管理页面支持"导入权限 JSON"入口，复制 [README 权限要求](../README.md#权限要求) 章节里的完整权限清单一次性粘贴即可开通 400+ 个 scope

**迁移指引**：

旧：
```bash
feishu-cli config add-scopes --domain all
```

新：
1. 打开飞书开放平台 → 你的应用 → 权限管理页面
2. 复制 README 的完整权限 JSON（tenant + user 两套 400+ scope）
3. 粘贴到"导入权限"入口，一键开通全部
4. 等待 tenant 管理员审批（如果需要）

**代码影响范围**：
- 删除 `cmd/config_add_scopes.go` 整个文件
- `cmd/auth_check.go` 的 `suggestion` 文案改为引导用户去开放平台开通（不再推荐 `config add-scopes`）
- README / CLAUDE.md / AGENTS.md / 6 个 skill 的 `config add-scopes` 引用全部更新为"去开放平台开通"
- 保留 `config create-app --save`（Device Flow 创建应用）不变

---

### Breaking Changes — 多维表格（bitable）切换到 `base/v3` API

**旧实现**：`bitable` 模块全部调用 `/open-apis/bitable/v1/apps/{app_token}/...` 老 API，覆盖 ~30 个基础 CRUD 命令。
**新实现**：全面切换到 `/open-apis/base/v3/bases/{base_token}/...` 新 API，覆盖 48 个命令，支持深度能力（视图完整配置读写、记录 upsert、修改历史、角色 CRUD、高级权限、数据聚合、工作流查询）。

#### 命令名迁移表

| 旧命令 | 新命令 |
|---|---|
| `bitable tables <app>` | `bitable table list --base-token <t>` |
| `bitable create-table <app>` | `bitable table create --base-token <t> --name x` |
| `bitable rename-table <app> <tbl>` | `bitable table update --base-token <t> --table-id <tbl> --name x` |
| `bitable delete-table <app> <tbl>` | `bitable table delete --base-token <t> --table-id <tbl>` |
| `bitable fields <app> <tbl>` | `bitable field list --base-token <t> --table-id <tbl>` |
| `bitable create-field` | `bitable field create` |
| `bitable update-field` | `bitable field update`（method 改为 `PUT`） |
| `bitable delete-field` | `bitable field delete` |
| `bitable records <app> <tbl>` | `bitable record list --base-token <t> --table-id <tbl>` |
| `bitable get-record` | `bitable record get` |
| `bitable add-record` | `bitable record upsert --base-token <t> --table-id <tbl> --config '...'` |
| `bitable add-records --data-file` | `bitable record batch-create --config-file ...` |
| `bitable update-record` | `bitable record upsert --record-id ...`（根据是否传 id 自动 PATCH/POST） |
| `bitable delete-records` | `bitable record delete --record-id ...` |
| `bitable views` | `bitable view list` |
| `bitable create-view` | `bitable view create` |
| `bitable delete-view` | `bitable view delete` |
| `bitable view-filter get/set` | `bitable view view-filter-get / view-filter-set` |
| `bitable dashboard list`（v1） | **暂不支持**（v3 dashboard CRUD 留待下次迭代） |
| `bitable form list` | **暂不支持** |
| `bitable role list` | `bitable role list`（新增 get/create/update/delete） |
| `bitable workflow list/enable` | `bitable workflow list`（改为 POST /workflows/list） |
| `bitable advperm enable/disable` | 同名但底层改为 `PUT .../advperm/enable?enable=true/false` |
| `bitable data-query` | 同名但路径从 table 级改为 base 级：`POST .../bases/{t}/data/query` |

#### 新增能力

- **视图配置完整写入**：`view-sort-set` / `view-group-set` / `view-visible-fields-set` / `view-timebar-set` / `view-card-set`（老 v1 只能写 filter）
- **记录修改历史**：`bitable record history-list --record-id xxx`
- **角色 CRUD**：`bitable role create/update/delete`（老 v1 只有 list）
- **字段选项搜索**：`bitable field search-options`
- **Base create 支持时区**：`--time-zone Asia/Shanghai`

#### Flag 变化
- **删除 `--app-token` 别名**：只保留 `--base-token`（与 base/v3 API 命名一致，不再做兼容别名）
- `bitable create` 的 `--description` 被删除（base/v3 不支持），新增 `--time-zone`
- `bitable data-query` 的 `--table-id` 被删除（v3 端点挂在 base 下）

#### 删除的文件
- `internal/client/bitable.go` / `bitable_test.go`（v1 实现）
- `cmd/bitable_create.go` / `bitable_get.go` / `bitable_copy.go` / `bitable_advperm.go` / `bitable_dashboard.go` / `bitable_data_query.go` / `bitable_form.go` / `bitable_record_upload_attachment.go` / `bitable_role.go` / `bitable_view_config.go` / `bitable_workflow.go`

#### 新增的文件
- `internal/client/base.go`（`BaseV3Call` + `BaseV3Path` helper + `X-App-Id` header 自动注入）
- `cmd/bitable_base.go` / `bitable_misc.go`（所有 base/v3 命令的注册）
- `cmd/bitable_table.go` / `bitable_field.go` / `bitable_record.go` / `bitable_view.go` 全部重写

---

### Breaking Changes — VC（视频会议）改造升级

- **`vc search`**：底层 API 从 `GET /meeting_list` 切换到 `POST /meetings/search`。
  - 新增 flag：`--query` / `--organizer-ids` / `--participant-ids` / `--room-ids`
  - 删除 flag：`--meeting-no` / `--meeting-status`
  - 必须指定至少一个过滤条件
- **`vc notes`**：
  - flag 从 `--meeting-id` / `--minute-token`（单数）改为 `--meeting-ids` / `--minute-tokens`（复数，支持 CSV 批量最多 50）
  - 新增第三路径 `--calendar-event-ids`：从日历事件自动反查会议 / 妙记
  - 新增开关 `--with-artifacts`（获取 AI 产物）/ `--download-transcript --output-dir`（下载逐字稿）
- **所有 vc / minutes 命令默认 User Access Token**，未登录时统一报错提示 `feishu-cli auth login`

#### 新增命令
- `vc recording --meeting-ids/-calendar-event-ids`：查询会议录制并自动提取 `minute_token`
- `minutes download --minute-tokens x,y,z --output ./dir`：批量下载妙记音视频媒体（SSRF 防护 / 重定向校验 / Content-Disposition 解析 / 文件名去重 / 5 req/s 速率限制 / `--url-only` 预览链接）
- `minutes get <token> --with-artifacts`：新增 AI 产物合并输出

---

### Added — `drive` 云盘命令组（8 个命令）

新增独立的 `drive` 子命令组，与现有 `file` / `media` / `doc media-*` 命令并存，提供增强能力：

| 命令 | 相比老命令的增强 |
|---|---|
| `drive upload` | 大文件自动分块（>20MB 走 `upload_prepare/part/finish` 三步式，每片独立重试 3 次；支持 User Token） |
| `drive download` | 流式下载 + 路径校验 + `--overwrite` / `--timeout` |
| `drive export` | 新增 **markdown 快捷路径**：docx → markdown 走 `/docs/v1/content` 直接拉取，不跑异步 export task；支持 sheet / bitable 按 `--sub-id` 导出 CSV；有界轮询（10×5s）+ 超时返回 resume 命令 |
| `drive export-download` | 通过 `file_token` 直接下载已完成的导出任务产物，配合 `drive export` 超时后接力完成 |
| `drive import` | **切换到 `/medias/upload_*` 端点 + `parent_type=ccm_import_open` + `extra` 字段**（不再在用户云盘留下中间文件）；格式特定大小限制（docx 20MB / sheet 20MB / bitable 100MB）；有界轮询 + resume |
| `drive move` | 文件夹移动自动轮询 `task_check`（30×2s），文件移动同步返回 |
| `drive add-comment` | 支持**富文本 `reply_elements`**（text / mention_user / link）+ `--block-id` 局部评论（docx）+ **wiki URL 自动解析**成 docx token |
| `drive task-result` | 通用异步任务查询（`--scenario import/export/task_check`），配合 drive export / import / move 的超时 resume |

**保留不动**：`file list / delete / mkdir / copy / shortcut / quota / meta / stats / version` + `media upload / download` + `doc media-download / media-insert` + `comment list / resolve / delete / reply`

---

### Added — `mail` 飞书邮箱模块（10 个命令，从零新建）

**全新命令组**。首期不支持附件和 CID 内联图片，仅支持纯文本和 HTML body。所有命令默认 User Access Token。

| 命令 | 功能 |
|---|---|
| `mail message --message-id x` | 获取单封邮件（`--format full/plain_text_full/raw`） |
| `mail messages --message-ids a,b,c` | 批量获取多封邮件 |
| `mail thread --thread-id x` | 获取邮件线程 |
| `mail triage` | 列出 / 搜索邮件（`--folder INBOX --label x --query xxx --unread-only --list-folders --list-labels`），`--query` 走专用 `POST /search` 端点 |
| `mail send` | 发送邮件（**默认保存为草稿**，加 `--confirm-send` 立即发送，安全兜底） |
| `mail draft-create` | 仅创建草稿 |
| `mail draft-edit --draft-id x` | 编辑已有草稿（全量覆盖） |
| `mail reply --message-id x --body "..."` | 回复邮件（自动 `Re: ` 前缀 + 引用块 + `In-Reply-To` / `References` header 继承） |
| `mail reply-all` | 全部回复（包含 To 和 CC，自动排除自己） |
| `mail forward --message-id x --to y` | 转发（自动 `Fwd: ` 前缀 + 原文正文引用） |

**关键技术点**：
- RFC 5322 EML 构建 + base64 URL-safe 编码，`POST /drafts` body `{"raw":"..."}`
- HTML 自动检测（`<html>/<div>/<b>/<br>` 等标签），可用 `--plain-text` / `--html` 强制
- 发件人地址默认从 `/user_mailboxes/{mailbox}/profile` 读取
- 地址格式支持 `"Name <email>"` 和 `"email"`
- Subject 去重：`reply` 自动避免 `Re: Re:`，`forward` 自动避免 `Fwd: Fwd:`

---

### Fixed

- **`mail reply` 引用块缺日期占位符**：之前的 quote header 模板第一个 `%s` 传空字符串，会输出 `"在 ，xxx 写道:"`，已修正为 `"{email} 写道:"`
- **分片上传 fd 泄漏**：`uploadFileMultipart` 之前每片每次重试都 `os.Open + Seek`，现改为外层打开一次 + `io.NewSectionReader`，大文件不稳定网络下重试时节省 N×syscall
- **`mail reply` 重复 `GetMailboxProfile` 调用**：之前在 `runMailReply` 里调用 2 次（一次取 selfEmail 一次取 from/fromName），现合并为 1 次，省 1 个 API RTT
- **`drive import` 上传端点错误**：之前走 `/files/upload_all` 会在用户云盘留下中间文件，现改为官方的 `/medias/upload_all` + `parent_type=ccm_import_open` + `extra`
- **`mail triage --query` 静默失效**：之前把 query 当 list 端点的查询参数，飞书会忽略；现改走专用的 `POST /search` 端点

### Refactor（内部代码清理，用户感知较小）

- 新增 `requireUserToken(cmd, cmdName)` helper，统一所有新命令的 "需要 User Access Token" 错误信息格式
- 删除重复的 `GetWikiNodeByToken`（58 行），改用已有的 `GetWikiNode`
- 删除 `internal/client/mail.go` 的 `joinPath`，用 `strings.Join`
- `dedupStrings` 从 `vc_recording.go` 移到 `vc_common.go`
- `runBaseV3WithJSON` 重构，抽出 `runBaseV3WithBody` 让命令层直接传已构造的 body
- `bitable view create/rename` 去掉 `cmd.Flags().Set("config", ...)` + `MarkHidden` 的 hack 模式
- 删除 `runBaseV3Simple` / `addBaseTokenFlag` / `exactlyOneNonEmpty` 三处死参数/死变量
- 所有文件统一 `gofmt`

---

## [v1.18.0] - 未发布

### Breaking Changes — OAuth 认证全面切换到 Device Flow

彻底删除 Authorization Code Flow，只保留 Device Flow（RFC 8628）。本地桌面、SSH 远程、容器、CI 全环境统一使用同一条命令，**无需任何重定向 URL 白名单配置**。

#### 删除的 flag

`auth login` 命令删除以下 flag：

- `--manual` — SSH 远程手动粘贴回调模式（Device Flow 下 SSH 和本地一视同仁）
- `--no-manual` — 强制本地回调模式（本地回调 HTTP server 已移除）
- `--port` — 本地回调端口（不再需要）
- `--print-url` — 非交互两步式第一步（改用 `--no-wait` + `--device-code`）
- `--method` — 授权方式选择（Device Flow 是唯一方式）
- `--scopes` — 请求 OAuth scope（飞书 token v2 端点实际忽略此参数，返回应用预配置的全部 scope）

#### 删除的子命令

- `auth callback <url> --state <state>` — Authorization Code Flow 换 token 专用，整体删除

#### 删除的代码

- `internal/auth/oauth.go`：`Login` / `loginLocal` / `loginManual` / `buildAuthURL` / `GenerateAuthURL` / `ParseCallbackURL` / `ExchangeToken` 等函数
- `internal/auth/browser.go`：`isLocalEnvironment()` 函数（曾在 macOS 上无条件返回 true 导致 SSH 远程 Darwin bug）
- `cmd/auth_callback.go`：整个文件

#### 修复的 bug

- **Issue #95**：飞书错误码 20029（重定向 URL 有误）。根因是 Authorization Code Flow 需要用户在飞书开放平台配置 `http://127.0.0.1:9768/callback` 白名单，Device Flow 直接绕过此要求
- **Darwin SSH bug**：`isLocalEnvironment()` 在 macOS 上无条件返回 `true`，SSH 到 Mac 服务器时错误走本地回调模式会 2 分钟超时失败。已通过删除该函数消除

### 新增 — `auth login` 的 JSON 事件流模式

- **`auth login --json`**：阻塞轮询 + JSON 事件流输出到 stdout。AI Agent 推荐配合 Claude Code 的 `run_in_background=true` 使用
  - 首次输出：`{"event":"device_authorization","verification_uri":"...","verification_uri_complete":"...","user_code":"...","device_code":"...","expires_in":240,"interval":5}`
  - 成功输出：`{"event":"authorization_success","expires_at":"...","refresh_expires_at":"...","scope":"..."}`

- **`auth login --no-wait --json`**：两步模式第一步。只请求 `device_code` 并立即输出 JSON，不启动轮询。适合 AI Agent 希望把"请求"和"轮询"拆到两次独立 Bash 调用的场景

- **`auth login --device-code <code> --json`**：两步模式第二步。用已有的 `device_code` 继续轮询直到授权完成

### 新增 — `auth check` 子命令

预检当前 Token 是否包含指定 scope，专为 AI Agent 在执行业务命令前做前置判断而设计：

```bash
feishu-cli auth check --scope "search:docs:read"
feishu-cli auth check --scope "search:docs:read im:message:readonly"
```

输出 JSON：

```json
{
  "ok": true,
  "granted": ["search:docs:read"],
  "missing": null
}
```

或失败情况：

```json
{
  "ok": false,
  "error": "not_logged_in",
  "missing": ["search:docs:read"],
  "suggestion": "feishu-cli auth login"
}
```

退出码 0 = 满足，非 0 = 缺少或未登录，AI Agent 可直接分支。

### 不变

- **Token 存储格式**：`~/.feishu-cli/token.json` 仍是明文 JSON，数据结构完全兼容。升级后**不需要重新登录**
- **Token 自动刷新**：`ResolveUserAccessToken()` 路径和 `RefreshAccessToken()` 逻辑保持不动，access_token 过期时用 refresh_token 自动刷新
- **`config create-app`** 命令完全不变（它本来就用 Device Flow）
- **`auth status`** / **`auth logout`** 行为不变
- **所有业务命令**（doc/msg/search/wiki/task/calendar/...）行为不变

### 迁移指引

#### 人类用户

无需任何迁移。一条命令通吃所有场景：

```bash
feishu-cli auth login
```

本地桌面会自动开浏览器，SSH 远程需要手动复制 stderr 里的链接在本机浏览器打开，一模一样的命令。

#### AI Agent / 脚本用户

旧的两步式：
```bash
feishu-cli auth login --print-url --scopes "..."
feishu-cli auth callback "<回调URL>" --state "<state>"
```

迁移为以下**任一**方案：

**方案 A（推荐）**：阻塞 + 后台运行：
```bash
# run_in_background=true
feishu-cli auth login --json
# 读 stdout 第一行拿 verification_uri_complete，展示给用户
# 等后台进程退出，读第二行 stdout 拿 authorization_success
```

**方案 B**：两步模式：
```bash
# 第一步
feishu-cli auth login --no-wait --json  # → device_code JSON
# 把链接展示给用户等待授权
# 第二步
feishu-cli auth login --device-code <code> --json  # → authorization_success
```

#### CI / 无头脚本

**Authorization Code Flow 本来就无法无头完成**（需要浏览器授权），Device Flow 同样需要人类介入一次。如果 CI 需要 User Token，应该预先在本地通过 `auth login` 拿到 token.json 然后把它作为 secret 部署到 CI 环境，**不需要任何迁移**。

### 详细对比

| 方面 | v1.17.0 及以前 | v1.18.0 |
|---|---|---|
| OAuth Flow | Authorization Code Flow（默认）+ Device Flow（`--method device`） | 仅 Device Flow |
| 子命令 | `login` / `callback` / `status` / `logout` | `login` / `check` / `status` / `logout` |
| `auth login` 的 flag | `--port` / `--manual` / `--no-manual` / `--print-url` / `--scopes` / `--method` | `--json` / `--no-wait` / `--device-code` |
| 重定向 URL 白名单 | 必须（Authorization Code Flow 前置条件） | 不需要 |
| SSH 远程支持 | 要么手动粘贴（`--manual`）要么非交互两步（`--print-url`） | 一条命令通吃 |
| AI Agent 非交互方案 | `--print-url` + `auth callback` | `--json` + `run_in_background` 或 `--no-wait` / `--device-code` |
| `offline_access` 注入 | 用户手动通过 `--scopes` 传 | CLI 强制注入，用户无需操心 |
| scope 预检 | 手动解析 `auth status` JSON 的 scope 字段 | `auth check --scope "..."` |

---

更早的版本请参考 [GitHub Releases](https://github.com/riba2534/feishu-cli/releases)。
