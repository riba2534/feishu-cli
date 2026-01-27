# 发送消息 content 结构体参考

## 通用说明

- `content` 必须是 JSON 字符串
- 需要换行时使用 `\n`
- 文本消息 `@` 语法与卡片 Markdown 不同

## text（文本）

```json
{
  "text": "test content"
}
```

补充：
- `@` 单个用户：`<at user_id="ou_xxx">Tom</at>`
- `@` 所有人：`<at user_id="all"></at>`
- 支持超链接 `[文本](链接)`
- 支持样式：`**加粗**`、`<i>斜体</i>`、`<u>下划线</u>`、`<s>删除线</s>`

## post（富文本）

使用 `md` 标签承载 Markdown：

```json
{
  "zh_cn": {
    "title": "我是一个标题",
    "content": [
      [
        {
          "tag": "md",
          "text": "**mention** <at user_id=\"ou_xxx\">Tom</at>\n- item1\n- item2"
        }
      ]
    ]
  }
}
```

结构说明：
- `zh_cn` / `en_us`：至少提供一种语言
- `content`：段落数组；每个段落是一个 node 列表
- 常用 `tag`：`text`、`a`、`at`、`img`、`media`、`emotion`、`hr`、`code_block`、`md`
- `md` 标签独占段落，适合直接承载 Markdown

## image（图片）

```json
{
  "image_key": "img_xxx"
}
```

## file（文件）

```json
{
  "file_key": "file_v2_xxx"
}
```

## audio（语音）

```json
{
  "file_key": "file_v2_xxx"
}
```

## media（视频）

```json
{
  "file_key": "file_v2_xxx",
  "image_key": "img_v2_xxx"
}
```

## sticker（表情包）

```json
{
  "file_key": "file_v2_xxx"
}
```

## share_chat（群名片）

```json
{
  "chat_id": "oc_xxx"
}
```

## share_user（个人名片）

```json
{
  "user_id": "ou_xxx"
}
```

## interactive（卡片）

方式一：card_id

```json
{
  "type": "card",
  "data": {
    "card_id": "7371713483664506900"
  }
}
```

方式二：template_id

```json
{
  "type": "template",
  "data": {
    "template_id": "xxxxxxxxxxxx",
    "template_version_name": "1.0.0",
    "template_variable": {
      "key1": "value1"
    }
  }
}
```

方式三：卡片 JSON

```json
{
  "schema": "2.0",
  "config": {
    "update_multi": true
  },
  "body": {
    "direction": "vertical",
    "elements": [
      {
        "tag": "markdown",
        "content": "示例内容"
      }
    ]
  }
}
```

## system（系统分割线）

```json
{
  "type": "divider",
  "params": {
    "divider_text": {
      "text": "新会话",
      "i18n_text": {
        "zh_CN": "新会话",
        "en_US": "New Session"
      }
    }
  },
  "options": {
    "need_rollup": true
  }
}
```
