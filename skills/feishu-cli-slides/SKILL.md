---
name: feishu-cli-slides
description: >-
  飞书 Slides 演示文稿。slides create 从 XML 模板创建演示文稿（POST /open-apis/slides_ai/v1/xml_presentations）；
  slides media-upload 上传媒体（drive upload_all + parent_type=slide_file，单文件 ≤20MB 不支持分片）。
  当用户请求"创建飞书 ppt"、"上传幻灯片"、"演示文稿"、"slides"时使用。
  不适用：复杂 slide 编辑（block insert/replace 复杂语义）暂未实现，走 lark-slides。
argument-hint: create | media-upload
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书 Slides 演示文稿技能

通过 `feishu-cli slides` 创建空白演示文稿，并把本地图片以 `slide_file` 媒体形式上传到该演示文稿，
返回的 `file_token` 可直接在 slide XML 中作为 `<img src="...">` 引用。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

> **范围声明**：本技能只覆盖「创建空白演示文稿」+「上传媒体」两个最小可用动作。如需在已有
> 演示文稿里做复杂 slide 编辑（block insert/replace 等），目前 CLI 未实现，请改走
> 官方 `lark-slides` 客户端或 OpenAPI 直调。

## 核心概念

### 两种产物分别是什么

| 命令 | 调用的 API | 产物 | 用途 |
|------|------------|------|------|
| `slides create` | `POST /open-apis/slides_ai/v1/xml_presentations` | `xml_presentation_id` | 后续所有 slides 操作的 ID（不是普通 docx token） |
| `slides media-upload` | `POST /open-apis/drive/v1/medias/upload_all` (`parent_type=slide_file`) | `file_token` | 可直接放进 slide XML 的 `<img src="...">` |

**关键约束**：
- `slide_file` 是 slides 后端唯一接受的 `parent_type`（lark-cli 实测：`slide_image` / `slides_image` /
  `slides_file` 都会被拒）
- `parent_node` 必须传 `xml_presentation_id`（而不是 docx token 或 file_token）
- 上传走单分片 `upload_all`，**不支持** `upload_prepare` 多分片，所以单文件硬限 20 MB

### XML 模板格式（create 内部）

`slides create` 在 CLI 内部用 `--title/--width/--height` 拼成最小可用 XML 模板再 POST：

```xml
<presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
  <title>演示文稿标题</title>
</presentation>
```

> 当前版本不暴露 `--xml-file` 参数让你直接传整个 presentation XML——只能通过 `--title/--width/--height`
> 影响这个最小模板。如需复杂初始内容，先 `create` 拿到 `xml_presentation_id`，再走 lark-slides
> 或后续 CLI 扩展。

## 前置条件

- **认证**：默认走 **App Token**（租户身份），通过 `--user-access-token` 或 `FEISHU_USER_ACCESS_TOKEN`
  可切换 User Token（推荐用 User Token，以个人身份创建，便于后续直接在飞书里编辑）
- **权限**：
  | 命令 | 所需 scope |
  |------|-----------|
  | `slides create` | `slides:presentation:create` 或 `slides:presentation:write_only` |
  | `slides media-upload` | `docs:document.media:upload` |
- **预检**：`feishu-cli auth check --scope "slides:presentation:create docs:document.media:upload"`

## 命令速查

### 1. `slides create` — 创建空白演示文稿

```bash
# 最简用法（默认尺寸 960x540，title="Untitled"）
feishu-cli slides create

# 指定标题
feishu-cli slides create --title "Q2 OKR"

# 自定义宽高（像素）
feishu-cli slides create --title "Wide Deck" --width 1920 --height 1080

# JSON 输出（脚本接力时常用，方便 jq 取 xml_presentation_id）
feishu-cli slides create --title "Demo" --output json

# 以用户身份创建（推荐，演示文稿归属个人）
feishu-cli slides create --title "Demo" --user-access-token <u-xxx>
```

**关键参数**：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--title`, `-t` | 演示文稿标题 | `Untitled` |
| `--width` | 幻灯片宽度（像素） | `960` |
| `--height` | 幻灯片高度（像素） | `540` |
| `--output`, `-o` | 输出格式（留空 = 文本摘要，`json` = JSON） | 文本摘要 |
| `--user-access-token` | 显式传 User Token | 不传走 App Token |

**返回**：

```
Slides 演示文稿已创建：
  xml_presentation_id: <id>
  title:               Q2 OKR
  revision_id:         1
```

`xml_presentation_id` 就是后续 `media-upload --presentation-token` 要传的值。

### 2. `slides media-upload` — 上传媒体到演示文稿

```bash
# 上传封面图
feishu-cli slides media-upload \
  --file ./cover.png \
  --presentation-token <xml_presentation_id>

# JSON 输出，方便 jq 接力拿 file_token
feishu-cli slides media-upload \
  --file ./cover.png \
  --presentation-token <xml_presentation_id> \
  --output json
```

**关键参数**：

| 参数 | 说明 | 必填 |
|------|------|------|
| `--file` | 本地图片路径（**≤ 20 MB**） | 是 |
| `--presentation-token` | 目标演示文稿的 `xml_presentation_id` | 是 |
| `--output`, `-o` | 输出格式（留空 = 文本摘要，`json` = JSON） | 否 |
| `--user-access-token` | 显式传 User Token | 否 |

**返回**：

```
图片上传成功：
  file_token:      <token>
  file_name:       cover.png
  size:            123456 bytes
  presentation_id: <xml_presentation_id>

提示: 在 slide XML 中可用作 <img src="<token>"/>
```

## 典型工作流

### 工作流 A：创建 + 上传封面图（端到端）

```bash
# 1. 创建空白演示文稿
PRES_ID=$(feishu-cli slides create --title "Q2 OKR" --output json | jq -r '.xml_presentation_id')

# 2. 上传封面图，拿到 file_token
FILE_TOKEN=$(feishu-cli slides media-upload \
  --file ./cover.png \
  --presentation-token "$PRES_ID" \
  --output json | jq -r '.file_token')

# 3. 现在 $FILE_TOKEN 可以拼到 slide XML 的 <img src="..."> 里
echo "presentation: $PRES_ID, cover: $FILE_TOKEN"
```

### 工作流 B：批量上传一组图片

```bash
PRES_ID=$(feishu-cli slides create --title "Photo Deck" --output json | jq -r '.xml_presentation_id')

for img in ./assets/*.png; do
  feishu-cli slides media-upload \
    --file "$img" \
    --presentation-token "$PRES_ID" \
    --output json | jq -r '"\(.file_name) -> \(.file_token)"'
done
```

## 何时转用其他工具

| 场景 | 改走 |
|------|------|
| 在已有演示文稿里插入/修改/删除 slide 或 block | `lark-slides`（官方 CLI）或 OpenAPI 直调 `slides_ai/v1/...` 编辑接口 |
| 直接传整个 presentation XML 模板 | 暂未暴露 `--xml-file`，等 CLI 扩展或 OpenAPI 直调 |
| 单文件 > 20 MB 的媒体 | 拆分小图，或直接走飞书客户端上传 |
| 把 markdown / docx 转成 slides | 暂不支持，建议先转 docx 再用飞书客户端导出 |
| 普通文档（不是演示文稿）上传媒体 | 走 **feishu-cli-drive** 技能（`drive upload`） |

## 注意事项

- **`parent_type` 不要乱改**：源码里硬编码 `slide_file`，且经 lark-cli 实测唯一可用值。
  自己改成 `slide_image` / `slides_image` / `slides_file` 都会被服务端拒绝
- **20 MB 上限不可绕过**：`upload_prepare` 多分片接口**不接受** `parent_type=slide_file`，
  CLI 在 client 侧也做了 20 MB 硬检查，超过会在本地直接报错（不会发请求）
- **`xml_presentation_id` ≠ docx token**：这是 slides 模块独立的标识符，不要拿去当
  `docx:document_id` 或 `drive:file_token` 用
- **User Token vs App Token**：默认 App Token（Bot 身份），演示文稿归 Bot 所有，普通人在
  飞书 UI 里看不到。**推荐传 `--user-access-token`** 让产物归个人，能直接在「我的空间」找到
- **图片格式**：常见 png/jpg/jpeg/gif/webp 等，由 `medias/upload_all` 自动推断 MIME

## 错误排查

| 错误 | 原因 | 解决 |
|------|------|------|
| `--file 不能为空` / `--presentation-token 不能为空` | 必填参数缺失 | 检查命令行参数 |
| `读取文件失败` / `--file 必须是普通文件` | 路径错或不是 regular file | 检查路径、不要传符号链接到目录 |
| `文件 X 大小 Y 字节超过 slides 上传限制（20 MB）` | 文件超过 20 MB | 压缩 / 切分，slides 后端硬限不可绕过 |
| `创建 slides 失败: code=99991663, msg=...` | scope 不够 | `auth check --scope "slides:presentation:create"` 然后重新 `auth login --domain slides --recommend` |
| `创建 slides 失败: HTTP 400, body: ...invalid xml_presentation...` | XML 模板异常（一般 title 含未转义字符触发） | CLI 已做 XML escape，若仍出现请 issue |
| 上传报 `parent_type invalid` 或类似 | 走的不是 `slide_file`（如直接 curl 调 medias/upload_all 时传错） | 用 CLI 而不是手敲 curl，CLI 已锁定 `slide_file` |

## 参考

- API 文档：飞书开放平台「智能演示」/「云文档 - 素材」
- 代码：`cmd/slides_create.go` / `cmd/slides_media_upload.go` / `internal/client/slides.go`
- 上游 PR：[feishu-cli#135](https://github.com/riba2534/feishu-cli/pull/135)
