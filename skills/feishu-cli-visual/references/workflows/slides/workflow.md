# 飞书 Slides 演示文稿技能

通过 `feishu-cli slides` 创建演示文稿、读取全文或单页 XML、逐页增删改、截图预览，以及上传图片。

**写任何 slide XML 之前先读** [`references/xml-schema-quick-ref.md`](references/xml-schema-quick-ref.md)：
`<slide>` 下只有 `<style>`/`<data>`/`<note>`，文字必须包在 `<content><p>…</p></content>` 里，
图片是 `<img>` 不是 `<image>`，坐标用 `topLeftX/topLeftY`，页面 960×540。

## 目录

- [身份与权限](#身份与权限)
- [命令速查](#命令速查)
- [创建](#1-创建-slides-create)
- [读取](#2-读取-slides-get别名-xml-get)
- [加页与删页](#3-加页与删页)
- [编辑已有页面](#4-编辑已有页面)
- [截图预览](#5-截图预览-slides-screenshot)
- [图片](#6-图片)
- [错误排查](#错误排查)

## 身份与权限

- **身份**：写命令（create / add-slide / delete-slide / replace-slide / update-slide / media-upload）默认 **Bot**，
  显式 `--user-access-token` 或 `FEISHU_USER_ACCESS_TOKEN` 切到用户身份；读命令（get / screenshot）优先 User Token、未配置回落 Bot。
  演示文稿通常属于用户本人：编辑用户已有的 PPT 时用 User 身份，Bot 没有该文件权限会直接报错。
  已登录时先取 User Token 再传入：`UAT=$(feishu-cli auth token --as user) && feishu-cli slides create ... --user-access-token "$UAT"`。
  不要把 `$(...)` 直接内联进参数：取 Token 失败时会传入空值，命令静默回落 Bot。
- **Bot 创建**：`slides create` 以 Bot 身份创建后自动给当前 CLI 登录用户授予 `full_access`，JSON 输出 `url` 与 `permission_grant`。
  应用未开通 tenant 的 `slides:presentation:create` 时报 99991672（exit 3，重新登录无效），改用 User 身份。
- **scope**：

| 命令 | 所需 scope |
|------|-----------|
| `slides create` | `slides:presentation:create` 或 `slides:presentation:write_only`（带图片另需 `docs:document.media:upload`） |
| `slides add-slide` / `delete-slide` / `replace-slide` / `update-slide` | `slides:presentation:update` 或 `slides:presentation:write_only` |
| `slides get` | `slides:presentation:read` |
| `slides screenshot` | `slides:presentation:screenshot` |
| `slides media-upload` | `docs:document.media:upload` |

传 `/wiki/` URL 时额外需要 `wiki:node:read`。预检：`feishu-cli auth check --scope "slides:presentation:update slides:presentation:read"`。

## 命令速查

所有接收演示文稿的命令都接受 `xml_presentation_id`、`/slides/` URL 或 `/wiki/` URL（wiki 自动解析，底层不是 slides 时报错）。
XML / JSON 参数（`--slide`、`--slides`、`--content`、`--parts`）都支持 `@file` 读文件、`-` 读 stdin，用来绕开 shell 转义——
多层转义是 3350001 的头号来源。写页面的命令都有 `--dry-run`。
只有 `create` / `get` / `media-upload` 有 `-o json`；`add-slide` / `delete-slide` / `replace-slide` / `update-slide` / `screenshot`
固定输出 JSON，没有 `-o`（传了会报 unknown flag，exit 2）。

| 场景 | 命令 |
|------|------|
| 新建（可带 ≤10 页） | `slides create --title T --slide @p1.xml --slide @p2.xml -o json` |
| 读全文 / 单页 XML | `slides get <id>`、`slides get <id> --slide-number 2 -o json` |
| 追加 / 插入一页 | `slides add-slide <id> --slide @page.xml [--before-slide-id <sid>]` |
| 删一页 | `slides delete-slide <id> --slide-id <sid> --yes` |
| 改单个元素 | `slides replace-slide <id> --slide-id <sid> --parts @parts.json` |
| 整页改写（换背景 / 批量改样式 / 删多个元素） | `slides update-slide <id> --slide-id <sid> --content @page.xml` |
| 截图核对 | `slides screenshot <id> --slide-number 1,2 --output-dir shots` |
| 上传图片拿 file_token | `slides media-upload --file ./a.png --presentation-token <id> -o json` |

## 1. 创建 `slides create`

```bash
# 空白演示文稿（960×540，title 默认 Untitled）
feishu-cli slides create --title "Q2 OKR" -o json

# 一步创建并带页面：每个 --slide 是一页（XML 或 @file），最多 10 页
feishu-cli slides create --title "Q2 OKR" --slide @cover.xml --slide @agenda.xml -o json

# 页面 JSON 数组（每项一个 <slide> XML 字符串）
feishu-cli slides create --title "Q2 OKR" --slides @slides.json -o json

# 预览请求
feishu-cli slides create --title "Q2 OKR" --slide @cover.xml --dry-run
```

- 流程：先建空白演示文稿 →（有 `@` 占位图时）上传图片 → 逐页添加。某页失败即停止，错误里给出已创建的
  `xml_presentation_id`、url 和已添加页数；**用 `slides add-slide` 补剩余页，不要重新 create**
- 页面在创建前就做结构校验（必须是单个完整 `<slide>` 根、不能带 `<?xml?>` 声明）、占位图片在创建前检查存在与 ≤20 MB，
  坏输入不会留下半成品演示文稿
- `--slide` 与 `--slides` 互斥；`--slides null` / 空值视为错误（防止命令替换失败时把空白 deck 报成成功）；超过 10 页先建再 add-slide
- JSON 输出：`xml_presentation_id`、`url`、`revision_id`、`slide_ids`、`slides_added`、`images_uploaded`；
  服务端对已写入页面有 schema 告警时另有 `slide_issues`
- **页面 XML 完全相同会被服务端去重**（实测）：重复的页不会新建，响应返回已有页的 `slide_id`。`create` 传两个相同的
  `--slide` 时 `slides_added=2` 但 `slide_ids` 重复、实际只有 1 页；需要相同版式的多页时让每页内容有差异
- `--width/--height` 只影响空白模板尺寸，默认 960×540

## 2. 读取 `slides get`（别名 `xml-get`）

```bash
feishu-cli slides get <id>                                # 全文 XML 打到 stdout
feishu-cli slides get <id> -o json                        # {xml_presentation_id, scope, revision_id, content}
feishu-cli slides get <id> --slide-number 2 -o json       # 单页（也可 --slide-id）
feishu-cli slides get <id> --slide-id <sid> --output-file page.xml   # 写文件，stdout 只给元信息
feishu-cli slides get <id> --remove-attr-id               # 去掉 id 属性，只读浏览用（仅全文读取；不能再按 id 编辑）
```

- 编辑前先读单页 XML，拿到页面 `slide_id` 和元素 `id`（`replace-slide` 的 `block_id`）
- `--revision-id` 默认 `-1`（最新）。实测读取接口**忽略正整数版本号、始终返回最新版本**，`0` 会被服务端以 3350001 拒绝，
  CLI 在本地直接报用法错误；需要历史版本请在飞书客户端的版本记录里查看

## 3. 加页与删页

```bash
feishu-cli slides add-slide <id> --slide @page.xml                       # 追加到最后
feishu-cli slides add-slide <id> --slide @page.xml --before-slide-id <sid>   # 插到某页之前
cat page.xml | feishu-cli slides add-slide <id> --slide -

feishu-cli slides delete-slide <id> --slide-id <sid> --dry-run
feishu-cli slides delete-slide <id> --slide-id <sid> --yes
```

- add-slide 一次一页；JSON 输出 `slide_id`、`revision_id`，有图片占位时 `images_uploaded`，有告警时 `issues`
- 与已有页 XML 完全相同的 add-slide 不会新建页（服务端去重，返回原 `slide_id` 和当时的 `revision_id`，
  `--before-slide-id` 也不生效）；返回的 `revision_id` 没有增长时回读确认
- delete-slide 是危险操作：交互终端会二次确认；非交互环境（Agent / 管道）必须显式 `--yes`，否则退出码 10 且不执行。
  删前先 `slides get --slide-id` 确认是哪一页；删除无法原地撤销，误删在飞书客户端版本记录中恢复
- add-slide / delete-slide / replace-slide 的 `--revision-id` 传具体版本号可做乐观锁（版本已变化时服务端拒绝）；
  update-slide 传旧版本号的语义不同：以该快照重建本页并丢弃之后的改动，通常保持默认 `-1`

## 4. 编辑已有页面

| 改动 | 用哪个 |
|------|--------|
| 改一个标题 / 文本块 / 图片，或插入一个元素 | `replace-slide`（只动点名的元素，同页其他元素不受影响） |
| 一页里改很多、换背景、删若干元素 | `update-slide`（整页覆盖，`slide_id` 与页序不变） |
| 多页大改 | 每页各跑一次 `update-slide`，不要重新 create 整份 PPT |

### replace-slide（元素级）

```bash
feishu-cli slides replace-slide <id> --slide-id <sid> --parts @parts.json
feishu-cli slides replace-slide <id> --slide-id <sid> --parts @parts.json --dry-run   # 看规范化后的真实请求
```

`parts.json`：

```json
[
  {"action": "block_replace", "block_id": "bkW",
   "replacement": "<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"600\" height=\"80\"><content fontSize=\"28\"><p>新标题</p></content></shape>"},
  {"action": "block_insert",
   "insertion": "<shape type=\"rect\" topLeftX=\"520\" topLeftY=\"440\" width=\"200\" height=\"60\"/>"}
]
```

CLI 自动处理（官方实测契约，避免 3350001）：

- `block_replace` 的 replacement 根元素自动注入 `id="<block_id>"`（实测不带 id 直接 3350001）
- 根为 `<shape>` 且缺 `<content/>` 时自动补上
- 兼容别名并在输出 `normalizations` 中列出：`replace`→`block_replace`、`insert`→`block_insert`、
  `target_id`→`block_id`、`block`/`content`/`element`/`shape`→`replacement`/`insertion`
- 不支持 `str_replace`（只做结构化编辑）；`page_replace`/`slide_replace` 提示改用 update-slide；单次最多 200 条
- 服务端部分失败时透出 `failed_part_index` / `failed_reason`（stderr 同时告警），已写入但有告警时透出 `issues`

### update-slide（整页覆盖）

```bash
feishu-cli slides get <id> --slide-id <sid> --output-file page.xml
# 编辑 page.xml（保留要留下的元素及其 id；不带 id 的元素会作为新元素插入）
feishu-cli slides update-slide <id> --slide-id <sid> --content @page.xml
```

- `--content` 里**没有的元素会被删除**，`<style>` 背景与 `<note>` 备注也按 XML 更新
- 根 `<slide>` 自动带上 `id=<slide-id>`；根上已有 id 但与 `--slide-id` 不同会被拒绝（防止把 A 页的 XML 写到 B 页）
- 自动去掉 `<note>` 上的 id：过期的 note id（从别页复制、或页面被重建过）会让服务端整页拒绝 `block is not NoteBlock`（实测 4001000）
- 服务端返回 `failed_reason` 表示整页没写进去，CLI 按失败退出（not found 时提示重新 get 当前 slide_id）

### 服务端 XML lint

写页面的命令（create 带页面、add-slide、replace-slide、update-slide）默认在请求体带 `lint_xml: true`，
由服务端对写入后的整页做版式检查，error 级问题以 **4000153** 拒绝写入（错误里给出报告与 error 数）。
确认是 lint 误判、页面必须原样提交时加 `--no-lint`。开关必须放在请求体：放 query 会被网关丢弃且不报错。
（当前租户实测服务端尚未拦截越界页面，lint 是否生效以服务端为准，提交前仍应自查坐标在 960×540 内。）

## 5. 截图预览 `slides screenshot`

```bash
feishu-cli slides screenshot <id> --slide-number 1 --output cover            # 自动补 .png/.jpg
feishu-cli slides screenshot <id> --slide-number 1,2,3 --output-dir shots    # 一次最多 10 页
feishu-cli slides screenshot <id> --slide-id <sid>                           # 默认目录 slides_screenshots
feishu-cli slides screenshot --content @page.xml --output preview            # 不写入演示文稿，直接渲染 XML
```

- stdout 只输出文件元信息（`screenshots[].path/format/size/slide_id/slide_number`），不打印 Base64；已有页面实测返回 JPEG，
  `--content` 模式返回 PNG
- 文件名 `<presentation>_p<三位页码>_<slide_id>.<ext>`（如 `_p001_`），同名自动加 `_2` 序号不覆盖；`--output` 只能对应一页，
  扩展名与实际格式不符时按实际格式改名
- `--content` 模式适合写回前预览效果；后续读取文件时以输出里的 `path` 为准

## 6. 图片

- `<img src>` 只能是上传到该演示文稿的 `file_token`，**不能用 http(s) 外链**（渲染端不代理外链）
- 两种方式：
  1. `slides media-upload --file ./a.png --presentation-token <id> -o json` 拿 `file_token` 再写进 XML（多页复用同一张图时用这个，只传一次）
  2. 在 create / add-slide / update-slide 的 XML 里写 `<img src="@./a.png" .../>`，CLI 先上传再替换；路径相对**当前工作目录**解析
- 单张 ≤ 20 MB（只能走单分片 `upload_all`）；`parent_type` 自动选择：原生演示文稿 `slide_file`，导入型 Office deck
  （token 以 `fake_office_`/`local_office_` 开头，或长度 ≥25 且第 5/10/15/20/25 位依次为 `OFL0X`）用 `office_slide_file`
- `<img>` 的 width:height 与原图比例不同会被自动裁剪

## 错误排查

| 错误 | 原因 | 处理 |
|------|------|------|
| `--revision-id 取值 0 无效` | 0 会被服务端拒绝 | 用 `-1`（最新） |
| `3350001 invalid param` | block_id/slide_id 不在当前页；XML 结构或元素非法；坐标越界 | 先 `slides get --slide-id` 回读最新 XML；对照 XML 速查修正 |
| `3350002 not found` | `--slide-id` / `--slide-number` 指向不存在的页 | 读全文确认当前页列表与页数 |
| `4000153` | 服务端 lint 拒绝 | 按报告修 error 级问题；确认误判再加 `--no-lint` |
| `4001000 ... block is not NoteBlock` | 整页 XML 带了过期 note id | 用 `update-slide`（CLI 自动剥除），或删掉 `<note>` 的 id |
| `99991672` | 应用缺 tenant scope（Bot 身份） | 用 User 身份，或让管理员为应用开通 |
| `99991679` | 用户未授权该 scope | `feishu-cli auth login --scope "<scope>"` 增量补授 |
| 退出码 10 | delete-slide 在非交互环境等待确认 | 向用户确认要删的页后再加 `--yes`，不要自行补 |
| `<presentation> 的资源类型是 "docx"` | 传了文档 URL | 换成 `/slides/` 或指向 slides 的 `/wiki/` 链接 |
| 图片不显示 | 写了外链或别的演示文稿的 token | 用 `@` 占位符或 media-upload 到当前演示文稿 |

## 参考

- XML 速查：[`references/xml-schema-quick-ref.md`](references/xml-schema-quick-ref.md)
