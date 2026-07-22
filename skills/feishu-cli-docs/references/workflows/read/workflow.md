# 飞书文档阅读技能

从飞书云文档、电子表格或知识库读取内容，转换为 Markdown 格式后进行分析和展示。普通电子表格使用 `sheet export --format markdown`，知识库 sheet 使用 `wiki export`。

## 目录

- [前置条件](#前置条件)
- [核心概念](#核心概念)
- [使用方法](#使用方法)
- [文档元信息和块](#获取文档元信息doc-get)
- [知识库读类](#知识库读类wiki-get--nodes--spaces)
- [电子表格读类](#电子表格读类sheet-read--list-sheets)
- [执行流程](#执行流程)
- [URL 格式](#支持的-url-格式)
- [错误处理](#错误处理与边界情况)

## 前置条件

- **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式
- 已完成认证（`feishu-cli auth login`）
- App 权限：需要 `docx:document` 或 `docx:document:readonly`（普通文档）、`wiki:wiki:readonly`（知识库）
- **Token 解析（所有读命令通用）**：`doc export` / `wiki export` / `sheet export` 等读类命令统一走"User 优先 + Tenant 兜底"——优先用 token.json 里的 User Token，未找到回落 App Token。所以读他人文档时只要 `auth login` 一次，后续不用再传 `--user-access-token`。详见下方"User Token 优先级链"小节。

## 核心概念

**Markdown 作为中间态**：本地文档与飞书云文档之间通过 Markdown 格式进行转换，中间文件存储在 `/tmp` 目录中。

## 使用方法

```bash
feishu-cli doc export <document_id> --output /tmp/feishu_doc.md --download-images --assets-dir /tmp/feishu_assets
feishu-cli wiki export <node_token_or_url> --output /tmp/feishu_wiki.md --download-images --assets-dir /tmp/feishu_assets
feishu-cli sheet export <spreadsheet_token_or_url> --format markdown --output /tmp/feishu_sheet.md
```

## 大文档选择性读取（doc read）⭐

**大文档不要整篇 export**——先看结构再取所需部分，节省上下文：

```bash
# 第一步：看标题大纲（层级缩进 + block_id）
feishu-cli doc read <document_id> --outline

# 第二步：只取目标章节（按标题子串匹配，输出到下一个同级/更高级标题前的 Markdown）
feishu-cli doc read <document_id> --heading "性能优化"

# 或直接按内容定位（正则，多词用 | 连接；--context 控制上下文行数，默认 3）
feishu-cli doc read <document_id> --keyword "QPS|限流" --context 5
```

三种模式互斥（三选一）。`--heading` 命中多个标题时输出第一个并在 stderr 提示其余候选；
代码块围栏内的 `#` 行不会被误判为标题。block 级精确分页仍用 `doc blocks`。

## 获取文档元信息（doc get）

读取文档基本信息（document_id、revision_id、title），用于在 export 之前确认目标、或拿 revision_id 作为后续 API 调用参数。同样走"User 优先 + Tenant 兜底"。

| Flag | 默认值 | 说明 |
| --- | --- | --- |
| `<document_id>` | 必填 | 文档 ID 或 URL（`https://xxx.feishu.cn/docx/<id>`） |
| `-o, --output` | text | 输出格式，可选 `json` |
| `--user-access-token` | 空 | 手动覆盖 User Token；不填则自动从 `~/.feishu-cli/token.json` 读取 |

```bash
# 文本摘要
feishu-cli doc get ABC123def456

# JSON 输出（脚本里拿 revision_id / title）
feishu-cli doc get ABC123def456 -o json

# 从 URL 直接读
feishu-cli doc get https://xxx.feishu.cn/docx/ABC123def456
```

## 列出文档所有块（doc blocks）

`doc export` 拿不到结构化块树时（例如要分析每个块的类型、定位特定块、查 raw API 响应），用 `doc blocks`。默认列出第一页（500 块），加 `--all` 自动分页拉完。

| Flag | 默认值 | 说明 |
| --- | --- | --- |
| `<document_id>` | 必填 | 文档 ID（不接 URL，请先 `doc get` 拿 ID） |
| `--all` | false | 自动分页获取所有块（覆盖 `--page-size` / `--page-token`） |
| `--page-size` | 500 | 单页块数量 |
| `--page-token` | 空 | 续页 token |
| `--document-revision-id` | -1 | 文档版本（-1 = 最新） |
| `--raw` | false | 输出飞书 API 原始 JSON（含未解析字段） |
| `--user-id-type` | open_id | 用户 ID 类型（open_id/union_id/user_id） |
| `-o, --output` | text | 输出格式，可选 `json`（CLI 归一化结构） |
| `--user-access-token` | 空 | 手动覆盖 User Token |

```bash
# 默认：第一页，文本摘要
feishu-cli doc blocks ABC123def456

# 全量分页 + 归一化 JSON
feishu-cli doc blocks ABC123def456 --all -o json

# 拿 API 原始响应（含未识别块类型的 raw 字段）
feishu-cli doc blocks ABC123def456 --all --raw > /tmp/blocks_raw.json
```

## 知识库读类（wiki get / nodes / spaces）

知识库的"目录结构遍历三件套"，配合 `wiki export` 完成"找到节点 → 读内容"的链路。三个命令都走"User 优先 + Tenant 兜底"。

### wiki get — 查节点元信息

| Flag | 默认值 | 说明 |
| --- | --- | --- |
| `<node_token \| url>` | 必填 | 节点 Token 或 wiki URL |
| `-o, --output` | text | 输出格式，可选 `json` |
| `--user-access-token` | 空 | 手动覆盖 User Token |

返回字段：`space_id` / `node_token` / `obj_token`（用于文档 API） / `obj_type`（docx/sheet/bitable/...） / `title` / `has_child`。

### wiki nodes — 列出空间或父节点的子节点

| Flag | 默认值 | 说明 |
| --- | --- | --- |
| `<space_id>` | 必填 | 知识空间 ID（由 `wiki get` 或 `wiki spaces` 得到） |
| `--parent` | 空 | 父节点 Token；不填 = 列空间根节点 |
| `--page-size` | 50 | 单页节点数量 |
| `-o, --output` | text | 输出格式，可选 `json` |
| `--user-access-token` | 空 | 手动覆盖 User Token |

### wiki spaces — 列出当前身份可见的所有知识空间

| Flag | 默认值 | 说明 |
| --- | --- | --- |
| `--page-size` | 50 | 单页空间数量 |
| `-o, --output` | text | 输出格式，可选 `json` |
| `--user-access-token` | 空 | 手动覆盖 User Token |

```bash
# 1. 列空间
feishu-cli wiki spaces

# 2. 看某节点信息，记下 space_id
feishu-cli wiki get https://xxx.feishu.cn/wiki/Ad8Iw0oz3iSp4kkIi7QctVhin3e

# 3. 列该节点下子文档
feishu-cli wiki nodes 7012345678901234567 --parent Ad8Iw0oz3iSp4kkIi7QctVhin3e

# 4. 找到目标后用 wiki export 读内容
feishu-cli wiki export <child_node_token> -o /tmp/child.md
```

## 电子表格读类（sheet read / list-sheets）

`sheet export --format markdown` 适合"整表导出阅读"；要按精确范围读单元格、或先列出工作表元信息，用下面两个命令。

### sheet list-sheets — 列出电子表格的所有工作表

| Flag | 默认值 | 说明 |
| --- | --- | --- |
| `<spreadsheet_token>` | 必填 | 电子表格 Token 或 URL |
| `-o, --output` | text | 输出格式，可选 `json` |
| `--user-access-token` | 空 | 手动覆盖 User Token |

返回 `sheet_id` / `title` / 索引 / 隐藏状态，配合 `sheet read` 的 `SheetID!A1:C10` 范围语法用。

### sheet read — 读指定范围单元格

| Flag | 默认值 | 说明 |
| --- | --- | --- |
| `<spreadsheet_token>` | 必填 | 电子表格 Token 或 URL |
| `<range>` | 必填 | 范围，例如 `SheetID!A1:C10`、`A1:B2`（配合 `--sheet-id`）、`Sheet1!A:C` 整列 |
| `--sheet-id` | 空 | 当 range 不带 SheetID 前缀时必填 |
| `--value-render` | 空 | 单元格值渲染：`ToString` / `FormattedValue` / `Formula` / `UnformattedValue` |
| `--datetime-render` | 空 | 日期渲染：`FormattedString`（不填返回数字时间戳） |
| `-o, --output` | text | 输出格式，可选 `json` |
| `--user-access-token` | 空 | 手动覆盖 User Token |

```bash
# 列出所有工作表
feishu-cli sheet list-sheets shtcnxxxxxx

# 读单个范围（推荐先 list-sheets 拿 sheet_id）
feishu-cli sheet read shtcnxxxxxx "0b12ab!A1:C10"

# 用工作表 ID 简化范围
feishu-cli sheet read shtcnxxxxxx "A1:C10" --sheet-id 0b12ab -o json

# 拿公式而非求值结果
feishu-cli sheet read shtcnxxxxxx "Sheet1!A1:B20" --value-render Formula
```

## 执行流程

1. **解析参数**

   - 判断 URL 类型：
     - `/docx/` → 普通文档，使用 `doc export`
     - `/wiki/` → 知识库文档，使用 `wiki export`
   - 如果是 Token，根据格式判断类型

2. **导出为 Markdown（含图片下载）**

   **普通文档**:

   ```bash
   feishu-cli doc export <document_id> --output /tmp/feishu_doc.md --download-images --assets-dir /tmp/feishu_assets
   ```

   文档内嵌电子表格块默认会自动展开为 Markdown 表格，便于直接阅读和分析；如果要保留 `<sheet .../>` 标签用于 roundtrip，追加 `--expand-sheets=false`。

   `doc export` 会自动解析 User Access Token（如已登录），解析优先级（与 `cmd/utils.go::resolveOptionalUserTokenWithFallback` + `internal/auth/resolve.go::ResolveUserAccessToken` 实现完全一致）：

   1. `--user-access-token` 命令行参数（若该 token 等于 token.json 中已过期的 access_token，且 refresh_token 仍有效，自动刷新）
   2. `FEISHU_USER_ACCESS_TOKEN` 环境变量（同样支持本机身份延伸的自动刷新）
   3. `~/.feishu-cli/token.json`（通过 `auth login` 保存；access_token 过期则用 refresh_token 自动续期并写回）
   4. `config.yaml` 中的 `user_access_token`（静态配置，不会自动刷新）
   5. **App Token 兜底**（资源 API 也会接受，以租户身份访问；遇到 1770032/forbidden 等错误时说明该文档对 App 不可见，必须走前 4 步拿到 User Token）

   找到 User Token 时使用用户身份访问，未找到或解析失败时回退为 App Access Token（租户身份）。

   若遇到 `code=1770032 forBidden`（App 无权限且未登录）或 `code=99991679 Unauthorized`（User Token 缺少 scope），需先在飞书开放平台为应用开通 `docx:document:readonly`，然后完成 User Token 授权：

   ```bash
   # 第一步：在飞书开放平台 → 你的应用 → 权限管理 → 搜索 docx:document:readonly → 开通
   # （或复制 README 的完整权限 JSON 一次性导入）
   feishu-cli auth login
   ```

   **知识库文档**:

   ```bash
   feishu-cli wiki export <node_token> --output /tmp/feishu_wiki.md --download-images --assets-dir /tmp/feishu_assets
   ```

   **普通电子表格**:

   ```bash
   feishu-cli sheet export <spreadsheet_token> --format markdown --output /tmp/feishu_sheet.md
   ```

   不指定 `--sheet-id` 时会读取所有可见工作表；只看单个工作表时加 `--sheet-id <sheet_id>`。

   **图片说明**：`--download-images` 仅属于 `doc export` / `wiki export`，用于把文档图片下载到本地。
   `sheet export` 没有该 flag；电子表格内图片需用 sheet 图片命令或导出 XLSX 后处理。

   **可选参数**：

   - `--user-access-token`：手动指定 User Access Token（不填则自动从 `~/.feishu-cli/token.json` 读取）
   - `--front-matter`：在 Markdown 顶部添加 YAML front matter（含标题和文档 ID）
   - `--highlight`：保留文本颜色和背景色（输出为 HTML `<span>` 标签）
   - `--expand-mentions`：展开 @用户为友好格式（默认开启，需要 contact:user.base:readonly 权限）
   - `--expand-sheets`：展开文档内嵌电子表格为 Markdown 表格（默认开启；设为 `false` 时保留 `<sheet .../>` 标签）

3. **读取文本内容**

   - 使用 Read 工具读取导出的 Markdown 文件
   - 分析文档结构和文本内容

4. **读取并理解图片内容**

   - 检查 `--assets-dir` 指定的目录是否有下载的图片
   - **使用 Read 工具逐个读取图片文件**（Claude 支持多模态，可直接理解图片内容）
   - 将图片内容整合到文档分析中

   ```bash
   # 列出下载的图片
   ls /tmp/feishu_assets/

   # 使用 Read 工具查看图片
   # Read /tmp/feishu_assets/image_1.png
   # Read /tmp/feishu_assets/image_2.png
   ```

5. **报告结果**
   - 提供文档摘要（包含图片内容描述）
   - 保留 Markdown 文件和图片供用户进一步操作

## 输出格式

向用户报告：

- 文档标题
- 文档结构概要（标题层级）
- 内容摘要（关键信息）
- 图片内容描述（如有图片）
- Markdown 文件路径（供后续使用）
- 图片文件路径（如有下载）

## 支持的 URL 格式

| URL 格式                                  | 类型     | 命令          |
| ----------------------------------------- | -------- | ------------- |
| `https://xxx.feishu.cn/docx/<id>`         | 普通文档 | `doc export`  |
| `https://xxx.feishu.cn/sheets/<token>`    | 普通电子表格 | `sheet export --format markdown` |
| `https://xxx.feishu.cn/wiki/<token>`      | 知识库（docx/sheet） | `wiki export` |
| `https://xxx.larkoffice.com/docx/<id>`    | 普通文档 | `doc export`  |
| `https://xxx.larkoffice.com/sheets/<token>` | 普通电子表格 | `sheet export --format markdown` |
| `https://xxx.larkoffice.com/wiki/<token>` | 知识库（docx/sheet） | `wiki export` |

## 示例

```bash
# 读取普通文档
feishu-cli doc export <document_id> --output /tmp/feishu_doc.md --download-images --assets-dir /tmp/feishu_assets
feishu-cli doc export https://xxx.feishu.cn/docx/<document_id> --output /tmp/feishu_doc.md

# 读取知识库文档
feishu-cli wiki export <node_token> --output /tmp/feishu_wiki.md --download-images --assets-dir /tmp/feishu_assets
feishu-cli wiki export https://xxx.feishu.cn/wiki/<node_token> --output /tmp/feishu_wiki.md

# 读取普通电子表格为 Markdown
feishu-cli sheet export <spreadsheet_token> --format markdown -o /tmp/feishu_sheet.md
```

## 导出格式说明

导出的 Markdown 支持以下飞书特有块类型的转换：

| 飞书块类型         | Markdown 表现                                          |
| ------------------ | ------------------------------------------------------ |
| Callout 高亮块     | `> [!NOTE]`、`> [!WARNING]` 等 6 种 GitHub-style alert |
| 块级/行内公式      | `$formula$`（LaTeX 格式）                              |
| 画板 (Board)       | `[画板/Whiteboard](feishu://board/...)` 链接           |
| 电子表格块 (Sheet) | 默认展开为 Markdown 表格；关闭 `--expand-sheets` 时输出 `<sheet .../>` |
| ISV 块 (Mermaid)   | 画板链接                                               |
| QuoteContainer     | `>` 引用语法（支持嵌套）                               |
| AddOns/SyncedBlock | 透明展开子块内容                                       |
| Iframe             | `<iframe>` HTML 标签                                   |

使用 `--highlight` 参数时，带颜色的文本输出为 `<span style="color:...">` 标签。

## 高级：Wiki 目录节点处理

知识库文档可能是**目录节点**（包含子节点），需要特殊处理。

### 1. 识别目录节点

当导出知识库文档时，如果 Markdown 内容显示为：

```markdown
[Wiki 目录 - 使用 'wiki nodes <space_id> --parent <node_token>' 获取子节点列表]
```

说明这是一个**Wiki 目录节点**（block_type=42），子文档列表存储在知识库元数据中。

### 2. 获取子节点列表

```bash
# 1. 先获取节点信息，记录 space_id
feishu-cli wiki get <node_token>

# 2. 列出该节点下的子节点
feishu-cli wiki nodes <space_id> --parent <node_token>
```

### 3. 完整处理流程

```bash
# 步骤 1：尝试导出文档
feishu-cli wiki export <node_token> -o /tmp/doc.md

# 步骤 2：检查内容
# 如果显示 "[Wiki 目录...]"，说明是目录节点

# 步骤 3：获取节点信息
feishu-cli wiki get <node_token>
# 记录 space_id 和 has_child 字段

# 步骤 4：获取子节点
feishu-cli wiki nodes <space_id> --parent <node_token>

# 步骤 5：逐个导出子节点
feishu-cli wiki export <child_node_token_1> -o /tmp/child1.md
feishu-cli wiki export <child_node_token_2> -o /tmp/child2.md
```

## 错误处理与边界情况

### 1. 常见错误

| 错误                              | 原因                                           | 解决                                                                                                        |
| --------------------------------- | ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `code=1770032, msg=forBidden`     | App Token 无权限访问该文档                     | 在飞书开放平台应用权限管理页面开通 `docx:document:readonly`，再 `auth login` 授权 User Token                 |
| `code=99991679, msg=Unauthorized` | User Token 缺少 `docx:document:readonly` scope | 在飞书开放平台应用权限管理页面开通 `docx:document:readonly`，再重新 `auth login`                             |
| `code=131002, param err`          | 参数错误                                       | 检查 token 格式                                                                                             |
| `code=131001, node not found`     | 节点不存在                                     | 检查 token 是否正确                                                                                         |
| `code=131003, no permission`      | 无权限访问                                     | 确认应用有 wiki:wiki:readonly 权限                                                                          |
| `code=131004, space not found`    | 知识空间不存在                                 | 检查 space_id 是否正确                                                                                      |
| 空内容或 `Unknown block type`     | 特殊块类型                                     | 见「高级：Wiki 目录节点处理」章节                                                                           |

### 2. 边界情况处理

**情况 1：文档内容为空**

- 检查文档是否真的为空
- 检查是否有权限查看内容
- 检查是否是目录节点（见上文）

**情况 2：图片下载失败**

- 检查 `--assets-dir` 目录是否可写
- 检查网络连接
- 图片可能已被删除或过期

**情况 3：部分块类型无法识别**

- 飞书 API 可能返回未知的块类型
- 这些块会显示为 `<!-- Unknown block type: XX -->`
- 这是正常现象，不影响其他内容的读取

**情况 4：大型文档**

- 超过 1000 个块的文档可能需要分页获取
- 使用 `feishu-cli doc blocks <doc_id> --all` 自动分页

### 3. 重试机制

如果遇到网络错误或 API 限流：

```bash
# 添加 --debug 查看详细错误信息
feishu-cli wiki export <token> --debug

# 等待几秒后重试
sleep 5 && feishu-cli wiki export <token>
```

## 注意事项

1. **识别目录节点**：目录节点的内容是子节点列表，不是实际文档内容
2. **公式内容**：导出的 LaTeX 公式保持原文，可直接被 Markdown 渲染器显示
3. **Callout 类型**：支持 NOTE/WARNING/TIP/CAUTION/IMPORTANT/SUCCESS 六种高亮块类型

## 常见问题

**Q: 提示权限不足 / `no permission` / `forBidden`**

- 确认应用已获得 `docx:document:readonly`（普通文档）或 `wiki:wiki:readonly`（知识库）权限
- 如果是他人文档且 App 没有被添加为协作者，需要使用 User Token：
  1. 在飞书开放平台 → 你的应用 → 权限管理 → 开通 `docx:document:readonly`
  2. 执行 `feishu-cli auth login`
  3. 授权后 `doc export` 会自动读取，无需额外参数

**Q: 文档不存在 / `node not found`**

- 检查文档 ID 或 node_token 是否正确（注意区分 `document_id` 和 `node_token`）
- 从 URL 中提取 ID 时确认使用了正确的路径段（`/docx/` 后为 document_id，`/wiki/` 后为 node_token）

**Q: Token 过期 / 认证失败**

- 运行 `feishu-cli auth status` 检查当前认证状态
- 如已过期，运行 `feishu-cli auth login` 重新认证
- 如使用 App Access Token，检查 `FEISHU_APP_ID` 和 `FEISHU_APP_SECRET` 环境变量是否正确
