# 卡片消息发送与历史排障

本文档只维护 `msg send` / `msg reply` 侧的卡片发送格式、`template_id` / `card_id` 用法和 v1 历史卡片排障。
卡片 JSON 的设计、组件、颜色与校验统一见 [`card` 工作流](../../card/workflow.md)（组件字段以
`../../card/references/components.md` 为准，设计规范见 `../../card/references/design.md`）。

## 目录

- [三种发送方式](#三种发送方式)
- [完整 Card JSON（v2）](#完整-card-jsonv2)
- [使用 template_id](#使用-template_id)
- [使用 card_id](#使用-card_id)
- [v1 历史卡片排障](#v1-历史卡片排障)
- [注意事项](#注意事项)

## 三种发送方式

| 方式 | content | 适用场景 |
|------|---------|---------|
| 完整 Card JSON 2.0 | `{"schema":"2.0","header":...,"body":...}` | Agent 动态生成卡片（推荐） |
| template_id | `{"type":"template","data":{"template_id":"...","template_variable":{...}}}` | 使用卡片搭建工具创建的模板 |
| card_id | `{"type":"card","data":{"card_id":"..."}}` | 引用已创建的卡片实体 |

三种都用 `--msg-type interactive` 发送。完整 Card JSON 先按 card 工作流生成并用 `lint_card.py` 校验；
`type=template` / `type=card` 是引用信封，不要送入只接受完整 Card JSON 2.0 的 linter，也不要擅自把它改造成 schema 2.0。

## 完整 Card JSON（v2）

最小结构，仅用于理解 `--content-file` 里应该放什么：

```json
{
  "schema": "2.0",
  "config": {"update_multi": true, "width_mode": "fill"},
  "header": {
    "template": "green",
    "title": {"tag": "plain_text", "content": "任务完成"}
  },
  "body": {
    "direction": "vertical",
    "elements": [
      {"tag": "markdown", "content": "所有子任务已完成，可以发布。"}
    ]
  }
}
```

```bash
# 从文件发送；卡片含本地图片路径时加 --upload-images，先用 --dry-run 核对请求体
feishu-cli msg send \
  --receive-id-type chat_id \
  --receive-id oc_xxx \
  --msg-type interactive \
  --content-file /tmp/card.json \
  --idempotency-key "card-001"

# 内联 JSON 发送简单卡片
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content '{"schema":"2.0","config":{"update_multi":true},"header":{"template":"blue","title":{"tag":"plain_text","content":"快速通知"}},"body":{"direction":"vertical","elements":[{"tag":"markdown","content":"任务已完成"}]}}'
```

## 使用 template_id

```bash
cat > /tmp/tpl.json << 'EOF'
{
  "type": "template",
  "data": {
    "template_id": "AAqk1xxxxxx",
    "template_version_name": "1.0.0",
    "template_variable": {
      "title": "部署通知",
      "env": "production",
      "version": "v1.2.3"
    }
  }
}
EOF

feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content-file /tmp/tpl.json
```

- `template_id` 必须来自用户提供或已授权的配置，并确认当前应用可用；`template_version_name` 可省略。
- 模板变量名需与模板定义一致；用模板发送时，模板本身的卡片数据也计入 30 KB 请求体上限。

## 使用 card_id

```bash
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content '{"type":"card","data":{"card_id":"7371713483664506900"}}'
```

`card_id` 是已创建的卡片实体 ID，必须来自本次请求或已授权上下文，示例值不能直接使用。

## v1 历史卡片排障

v1 卡片只用于读懂旧消息或迁移，**新增卡片一律用 v2**（迁移清单见 `../../card/references/v2-vs-v1.md`）。

| 特征 | v1 | v2 |
|------|----|----|
| 顶层容器 | 顶层 `elements` 数组 | `schema: "2.0"` + `body.elements` |
| 按钮 / 备注 | `action` 容器、`note` | 按钮直接放在 `body.elements`，无 `note` |
| 表格 / 图表 / 表单 | 不支持 | `table` / `chart` / `form` |
| Markdown | `lark_md`：不支持标题、列表、表格，`<font>` 颜色有限 | CommonMark + `<font>` 全色枚举 |

```json
{
  "header": {"template": "blue", "title": {"tag": "plain_text", "content": "卡片标题"}},
  "elements": [
    {"tag": "markdown", "content": "内容"},
    {"tag": "hr"},
    {"tag": "note", "elements": [{"tag": "plain_text", "content": "备注"}]}
  ]
}
```

- 卡片 Markdown 里 @ 人写 `<at id=ou_xxx></at>` / `<at id=all></at>`（无引号）；text/post 消息写
  `<at user_id="ou_xxx">`（有引号），两者不同。
- 解析历史消息里的 v1 卡片：内容在顶层 `elements[]`（不在 `body` 里），见
  `../../chat/references/output-quirks.md` §8。

## 注意事项

1. **大小限制**：卡片请求体最大 30 KB（超限返回 230025；template_id 卡片按模板数据计），超出时精简内容或拆分多条消息
2. **按钮回调**：`url` 属性可直接跳转（无需服务端）；`value` 回调需要应用接收 `card.action.trigger`（可用 `../../event/workflow.md` 的 `event consume card.action.trigger` 消费）
3. **图片引用**：`img_key` 不能直接写外部 URL；本地图片写路径并在发送时加 `--upload-images`，CLI 会上传后替换为 key
4. **v1 vs v2**：新增卡片用 v2；v1 仅用于历史兼容排查
5. **颜色语义**：header 颜色应与消息语义匹配（绿=成功、红=错误、橙=警告、蓝=通知），完整枚举见 `../../card/references/components.md`
