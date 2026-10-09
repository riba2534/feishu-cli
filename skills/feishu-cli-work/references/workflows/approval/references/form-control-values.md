# 审批实例表单控件 value 速查

> 创建审批实例（`feishu-cli approval instance create --form`）时，`--form` 是控件 JSON 数组，每个控件形如 `{"id":"widget1","type":"input","value":"..."}`。本文速查各控件 `type` 与 `value` 结构，是发起审批填表单的唯一依据。

## 通用参数

每个控件都含：

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | string | 是 | 控件 ID，需与审批定义中的控件 ID 一致 |
| `type` | string | 是 | 控件类型，见下表 |
| `value` | 因控件而异 | 是 | 控件取值，结构见下文 |

> 先用 `feishu-cli approval get <code> --output raw-json` 拿审批定义的 `form` 参数，确认各控件 `id` 和可选值范围。

## 控件 value 速查表

| 控件 | type | value 结构 |
|---|---|---|
| 单行文本 | `input` | `string` |
| 多行文本 | `textarea` | `string` |
| 日期 | `date` | RFC3339 `string`（如 `2019-10-01T08:12:01+08:00`） |
| 日期区间 | `dateInterval` | `{start, end, interval}`（均 RFC3339） |
| 单选 | `radio` / `radioV2` | `[option_value]`（string 数组） |
| 多选 | `checkbox` / `checkboxV2` | `[option_value, ...]` |
| 数字 | `number` | `float`（如 `1234.5678`） |
| 金额 | `amount` | `float` + 同级 `currency`（如 `"USD"`） |
| 联系人 | `contact` | `open_ids=[open_id]`（官方推荐只传 open_ids；也可同时传 `value=[user_id]`） |
| 关联审批/文档 | `document` | `{token, type}`（type: docx/sheet/bitable...） |
| 附件 | `attachmentV2` | `[file_code]`（审批文件 code，需用户提供） |
| 图片 | `image` | `[file_code]`（审批文件 code，需用户提供） |
| 明细/字段列表 | `fieldList` | `[[{id,type,value}, ...], ...]`（二维数组，每组一行） |
| 部门 | `department` | `[{open_id:"od-xxx"}]`（对象数组） |

## JSON 示例

### 文本类

```json
[{"id":"widget1","type":"input","value":"张三"},
 {"id":"widget2","type":"textarea","value":"多行\n说明"}]
```

### 日期 / 日期区间

```json
[{"id":"widget1","type":"date","value":"2024-01-15T09:00:00+08:00"},
 {"id":"widget2","type":"dateInterval","value":{"start":"2024-01-15T09:00:00+08:00","end":"2024-01-18T18:00:00+08:00","interval":3.0}}]
```

### 单选 / 多选（option value 从审批定义 form 拿）

```json
[{"id":"widget1","type":"radioV2","value":["option_1"]},
 {"id":"widget2","type":"checkboxV2","value":["option_1","option_2"]}]
```

### 数字 / 金额

```json
[{"id":"widget1","type":"number","value":1234.5678},
 {"id":"widget2","type":"amount","value":999.99,"currency":"CNY"}]
```

### 联系人 / 部门

```json
[{"id":"widget1","type":"contact","open_ids":["ou_xxx"]},
 {"id":"widget2","type":"department","value":[{"open_id":"od-xxx"}]}]
```

### 附件 / 图片 / 文档（file code 由用户提供，文档 token 可从链接提取）

```json
[{"id":"widget1","type":"attachmentV2","value":["D93653C3-2609-4EE0-8041-61DC1D84F0B5"]},
 {"id":"widget2","type":"image","value":["<file_code>"]},
 {"id":"widget3","type":"document","value":{"token":"<docx_token>","type":"docx"}}]
```

### 明细 fieldList（二维数组，每组是一个明细行的控件集合）

```json
[{"id":"widget1","type":"fieldList","value":[
  [{"id":"widget1","type":"input","value":"行1列1"},
   {"id":"widget2","type":"number","value":10}],
  [{"id":"widget1","type":"input","value":"行2列1"},
   {"id":"widget2","type":"number","value":20}]
]}]
```

## 取值来源

- **option value（单选/多选）**：从审批定义 `form` 中控件的 `option.value` 拿；关联外部选项时用 `options.id`
- **currency（金额）**：从审批定义金额控件的 `value` 拿可设置的货币种类
- **file code（附件 attachmentV2 / 图片 image）**：本 CLI 未封装审批文件上传，需用户直接提供 file code；
  不要用 `feishu-cli media upload`（它上传的是文档素材）。拿不到 file code 时告诉用户该控件无法仅靠 CLI 提单
- **open_id（联系人）**：用户给姓名/邮箱/手机号时，先用 `feishu-cli user search --email user@example.com`
  （或 `--mobile` / `--query`，见 feishu-cli-platform 的 directory 工作流）换成 `ou_` open_id
- **部门 open_department_id（`od-` 开头）**：优先让用户直接提供；有多个候选部门时让用户选，不要猜
- **文档 token**：从飞书文档链接中提取（`/docx/<token>`）

## API 不支持的控件

创建审批实例 API 不支持以下控件，必须用这些控件时不能仅靠本 API 提单：

| 控件/控件组 | type |
|---|---|
| 流水号 | `serialNumber` |
| 出差控件组 | `tripGroup` |
| 录用控件组 | `apaascorehrOnboardingGroup` |
| 转正控件组 | `apaascorehrRegularateGroup` |
| 补卡控件组 | `remedyGroupV2` |
| 调岗控件组 | `apaascorehrJobAdjustGroup` |
| 离职控件组 | `apaascorehrOffboardingGroup` |

## 参考

- 完整控件参数：飞书开放平台「审批实例表单控件参数」文档
- 发起审批工作流：本 skill 的 `approval instance create` 命令
