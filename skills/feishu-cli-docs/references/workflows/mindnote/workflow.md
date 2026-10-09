# 飞书思维笔记节点（mindnote nodes）

读取**已有**思维笔记（Mindnote）的节点，在其中新增子节点或按 `node_id` 更新已有节点。

这条链路**不新建思维笔记**：`mindnote nodes create` 是新增/更新节点，不是创建一个新的思维笔记文档。
用户要"新建一张思维导图"时，改用画板（`feishu-cli-visual` 的 board 工作流，Mermaid `mindmap` 可导入为画板），
或随 Markdown 导入（`../import/workflow.md`）。

## 获取思维笔记 token

`mindnote nodes` 的位置参数（或 `--mindnote-id`）接受以下输入：

| 输入 | 处理 |
|---|---|
| 思维笔记 token | 直接使用 |
| `https://xxx.feishu.cn/mindnotes/<token>` | 取路径中的 token |
| `https://xxx.feishu.cn/wiki/<node_token>` | 自动 `node_by_token` 解包；底层类型不是 mindnote 时以退出码 2 报错，不调用思维笔记接口 |
| `/docx/`、`/sheets/` 等其他类型链接 | 本地拒绝，退出码 2 |

- 不要把 wiki 链接里的 `node_token` 当裸 token 传：裸 token 一律按思维笔记 token 处理，服务端会返回 `3410003 resource not found`。
  传完整 `/wiki/` URL，或先 `feishu-cli drive inspect --url "<链接>"` 确认底层类型是 mindnote 再取真实 token。
- 位置参数与 `--mindnote-id` 二选一；两者都给且不一致时报用法错误。

## 身份与权限

| 命令 | 默认身份 | 可选 | 所需 scope |
|---|---|---|---|
| `mindnote nodes list` | `--as user` | `auto` / `bot` | `mindnote:node:read` |
| `mindnote nodes create` | `--as auto`（User 优先，未配置回退 Bot） | `user` / `bot` | `mindnote:node:create` |

- 官方元数据中 list 只声明 User 身份，因此默认 `user`；`--as bot` 时网关按应用身份 scope 校验（应用未开通返回 99991672）。
- 应用未开通 scope 报 `99991672`（退出码 3，需应用管理员在开放平台开通并发布）；User 未授权报 `99991679`
  （退出码 3，`feishu-cli auth login --scope "mindnote:node:read mindnote:node:create"`）。先 `feishu-cli auth check --scope "mindnote:node:read"` 预检。
- `--user-id-type open_id|union_id|user_id` 控制 @用户元素中的 ID 类型，不传由服务端按 open_id 处理。

## 读取节点

```bash
# 缩进节点树：文本 + [node_id]，附注已完成 / 高亮 / 图片数，备注单独一行
feishu-cli mindnote nodes list "https://xxx.feishu.cn/mindnotes/<mindnote_token>"

# 接口原始 data（nodes[].node_id / parent_id / texts / notes / images / finish / highlight），写入前用它确认 ID
feishu-cli mindnote nodes list --mindnote-id "https://xxx.feishu.cn/wiki/<node_token>" -o json
```

父节点不在返回列表中的节点按根节点展示。富文本元素按 `element_type`（text / link / user / doc）尽力拼成纯文本；
需要精确结构时用 `-o json`。

## 新增与更新节点

`--data` 接受内联 JSON、`@文件` 或 `-`（stdin）。请求体字段：

| 字段 | 说明 |
|---|---|
| `client_token` | 幂等 token；也可用 `--client-token` 传入（两处不一致时报用法错误） |
| `nodes` | 必填且非空的节点数组 |
| `nodes[].parent_id` | 在该节点下新增子节点 |
| `nodes[].node_id` | 指向已有节点时表示更新该节点 |
| `nodes[].texts` / `notes` | 富文本元素数组，最常见 `[{"element_type":"text","text":{"content":"内容"}}]` |
| `nodes[].images` | `[{"token":"<图片 token>"}]`，是已上传图片的 token，不是本地路径或 URL |
| `nodes[].highlight` | `red` / `yellow` / `pink` / `blue` / `cyan` / `olive` / `grey`（其他值本地拒绝） |
| `nodes[].finish` | 完成状态 |

```bash
# 先预览请求（不联网、不解析身份；wiki 链接显示解包步骤和占位 token）
feishu-cli mindnote nodes create "<mindnote_token>" --dry-run \
  --data '{"nodes":[{"parent_id":"node_parent123","texts":[{"element_type":"text","text":{"content":"子节点内容"}}],"highlight":"yellow"}]}'

# 新增子节点：生成一次 client_token，重试时复用同一个值
CT=$(uuidgen)
feishu-cli mindnote nodes create "<mindnote_token>" --client-token "$CT" \
  --data '{"nodes":[{"parent_id":"node_parent123","texts":[{"element_type":"text","text":{"content":"子节点内容"}}]}]}'

# 更新已有节点（文本、高亮、完成状态）
feishu-cli mindnote nodes create "<mindnote_token>" --client-token "$CT" \
  --data '{"nodes":[{"node_id":"node_existing123","texts":[{"element_type":"text","text":{"content":"更新后的内容"}}],"highlight":"blue","finish":true}]}'

# 复杂请求体放文件
feishu-cli mindnote nodes create "<mindnote_token>" --data @nodes.json -o json
```

- CLI **不会自动生成** `client_token`（与官方一致）：每次运行生成新值无法让重试幂等。未提供时 stderr 提示；
  需要安全重试时自己生成并在重试中复用。
- 输出：文本模式列出提交的节点数、返回的节点 ID 和 `client_token`；`-o json` 输出接口原始 data（`ids`、`client_token`）。

## 推荐流程

1. 判断目标：新建思维笔记 → 改走画板；操作已有思维笔记 → 继续。
2. 解析 token：wiki 链接直接传完整 URL（CLI 自动解包并校验类型），不确定类型时先 `drive inspect --url`。
3. `mindnote nodes list ... -o json` 确认目标 `parent_id` / `node_id`。
4. `mindnote nodes create ... --dry-run` 预览请求体；确认插入位置或被更新的节点无误。
5. 带 `--client-token` 正式执行；结果用 `mindnote nodes list` 复核。

## 错误处理

| 错误 | 原因 | 处理 |
|---|---|---|
| 退出码 2：`不是思维笔记（mindnote）` | wiki 节点底层是 docx/sheet 等 | 按真实类型改用对应命令 |
| 退出码 2：`仅支持 mindnote` / `不接受部分路径` | 传了其他类型链接或残缺路径 | 传思维笔记 token、`/mindnotes/` 或 `/wiki/` 完整链接 |
| 退出码 2：`--data...` | 请求体不是对象、`nodes` 为空、highlight 非法、client_token 冲突 | 按提示修正 `--data` |
| `99992402 mindnote_id: the min len is 20` | token 被截断或传错 | 检查 token 是否完整 |
| `3410003 resource not found` | token 不存在、当前身份无权访问，或把 wiki node_token 当成思维笔记 token | 传完整 `/wiki/` URL 或先 `drive inspect`；确认身份有权限 |
| `99991672` / `99991679`（退出码 3） | 应用未开通 / 用户未授权 `mindnote:node:*` | 见上方「身份与权限」 |
