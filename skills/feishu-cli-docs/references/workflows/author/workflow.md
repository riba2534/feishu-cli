<!-- 内容改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.） -->
# 从零创作飞书文档

用户要求"写 / 起草 / 生成"一篇文档（方案、PRD、周报、会议纪要、数据报告、教程、公众号文章等）时使用本工作流。
只建空文档、原样导入用户已给出的完整内容、或修改已有文档时不走本工作流，见 `../write/workflow.md` / `../import/workflow.md`。

**从零创作必须按下面的步骤依次执行，简单任务也不跳步。** 每个参考文件只在首次进入对应步骤时读取一次。

## 原则

以下原则是每个内容、结构和视觉决策的判定依据；写作和复查时逐条套用，冲突时按"约束栈"排序。

- **读者本位**：落笔前先回答读者是谁、为什么读、带着什么任务来；按读者任务组织内容，不按功能或作者视角罗列。
- **结构先行**：结论先行，先整体后局部；按逻辑分组与递进，依据关系选择列表、步骤或表格，便于扫读（特殊体裁除外）。
- **视觉服从语义**：先确定全篇主线和每节的中心任务，再让视觉层级复现内容优先级；文档脱离讲解仍须完整、连续、可独立阅读。
- **最低理解成本**：选最能降低读者理解、执行和出错成本的表达，而不是字符最少或制作最省事的；删冗余，用短句、动词和数据，
  按真实信息关系使用图、表格或交互组件。
- **克制且连贯**：每个视觉元素都要承担导航、比较、解释、证据、行动或体裁所需的氛围功能；相关文字与视觉相邻，同类关系复用同类组件。
  去掉后不影响读者任务的装饰应删除。
- **约束栈**：事实 > 用户硬约束 > 读者任务 > 内容 > 组件样式；后项不得牺牲前项，格式与组件不得反向改变内容判断。
- **表达一致**：同一对象、动作和状态全文同名；标题层级与编号二选一并贯穿全文：
  - **自动编号**：每个正文标题写 `seq="auto"`，标题文本不手写序号；
  - **中文手写**：公文或正式场景在标题文本中手写 `一、→（一）→ 1.→（1）`，绝不出现 `一、` 下接 `1.1`。

## 步骤

### Step 1：理解读者任务、格式要求、硬约束和禁区

### Step 2：选择体裁（genre contract）

以下文件位于 `references/genres/`。路由表只用于选候选；高置信命中后**必须读取**对应路由文件并按其中的消歧规则复核，
确认后最多各读一个 contract 与一个 adapter，记下固定短名；未命中时可省略。contract 决定内容任务、证据和体裁边界，
adapter 只调整与 contract 兼容的平台结构、文风和组件约束。

| 内容路由 | 独特任务 |
|---|---|
| [`route-workplace.md`](references/genres/route-workplace.md) | 组织决策、执行、留档（纪要、周报、提案、PRD、技术文档、SOP 等） |
| [`route-report.md`](references/genres/route-report.md) | 用数据、研究和证据形成洞察 |
| [`route-knowledge.md`](references/genres/route-knowledge.md) | 理解、自学、一次已知操作或检索 |
| [`route-media.md`](references/genres/route-media.md) | 独立采集、核实和公共理解 |
| [`route-opinion.md`](references/genres/route-opinion.md) | 形成并论证判断 |
| [`route-consumer.md`](references/genres/route-consumer.md) | 以真实体验或测试辅助消费选择 |
| [`route-marketing.md`](references/genres/route-marketing.md) | 组织授权的认知、转化或公关内容 |
| [`route-personal-brand.md`](references/genres/route-personal-brand.md) | 本人经历、能力和作品的可信呈现 |
| [`route-creative.md`](references/genres/route-creative.md) | 角色、冲突、情节与分支叙事 |

| Adapter 路由 | 渠道 |
|---|---|
| [`route-platform.md`](references/genres/route-platform.md) | 邮件、微信公众号、小红书成稿 |

### Step 3：收集资料并扫描表达机会

1. 扫描事实、数据、案例、引用和图片等资源缺口；内容需要而现有材料不足时必须检索或生成，需要图片且用户未提供时先搜索可用图片。
2. 按用户要求、contract / adapter 限制和内容需要选择表达方式，不因组件存在就机械使用：

   | 信息关系 | 候选表达 |
   |---|---|
   | 同组字段的精确比较或映射 | `table` |
   | 流程、依赖、分支、时序、层级、因果、空间或拓扑关系 | `whiteboard` |
   | 对象、场景、界面、外观、氛围、示例或视觉证据 | `img` |
   | 复杂交互、动态状态、可探索数据或应用式布局 | `html5-block` |
   | 两组简短、等权且适合横向阅读的信息 | `grid` |
   | 单个关键提醒或限制 | `callout` |
   | 简单并列、步骤或连续论述 | 列表或段落 |

3. 按全篇、章节、块三个尺度构图：相关内容相邻，同类关系保持相同顺序与对齐；正文可以是主表达，不要求每节都有展示块。
4. 写正文前确定计划使用的展示块。Presentation Decision 的 `visual_plan.blocks` 只记录确需最低数量约束的块（通常是
   `whiteboard`、`img`、`table`）；没有硬性数量要求时写 `"blocks": []`。

`presentation_mode` 记录视觉策略；只有用户要求与 contract / adapter 限制冲突时才询问用户：

- `formal`：正式、克制；不用高亮块、emoji 或装饰性组件，只保留正式体裁确有必要的结构。
- `normal`：按内容需要使用组件；只有能降低理解、执行或出错成本时才扩展视觉表达。
- `rich`：主动利用图片、画板、HTML 组件和其他飞书组件；每个组件须有明确目的，不设数量配额。

### Step 4：提交 Presentation Decision 并初始化草稿

生成完整 JSON，字段值必须来自 Step 1–3，不得照抄示例。`word_count` 只在用户明确提出字数要求时加入（"约 N 字"按 ±10%，
单边不限写 `null`），无要求时省略整个字段：

```json
{
  "audience": "项目负责人",
  "reader_task": "判断偏差并决定下一轮动作",
  "genre_contract": "weekly-report",
  "adapter": null,
  "presentation_mode": "rich",
  "visual_plan": {
    "reason": "需要用因果图解释偏差来源与后续行动依赖",
    "blocks": [
      {"type": "whiteboard", "min_count": 1, "purpose": "展示偏差成因与行动依赖"}
    ]
  }
}
```

不要预建临时目录、草稿或决策文件。把 JSON 原样代入并实际执行（JSON 含单引号或较长时先存为文件，用 `"@./decision.json"` 传入）：

```bash
feishu-cli doc script --command init-draft --presentation-decision '<上方完整 JSON>'
```

- 输出顶层的 `cwd`、`workspace`、`draft_path`：后续命令都在 `cwd` 下执行；把 `workspace` 记为 `work_dir`，`draft_path`
  （已含工作区前缀）记为草稿路径。
- CLI 已创建 `work_dir` 并保存 `.presentation-decision.json` 作为固定基线，**但没有创建 `draft_path` 指向的 XML**；
  首次写入前不要读取它。参数细节与报错见 [`doc-script.md`](references/doc-script.md)。

### Step 5：生成完整草稿（release candidate）

读取 [`docx-xml.md`](references/docx-xml.md)，结合 Presentation Decision、所选 contract 和上面的原则生成完整 DocxXML；
用到扩展标签时按需读取 [`docx-xml-extended-blocks.md`](references/docx-xml-extended-blocks.md)。

1. XML 以唯一的 `<title>` 开头，直接写入 `<cwd>/<draft_path>`。新资源放在 `<cwd>/<work_dir>` 下，已有资源原地复用；
   当前目录内优先用 `@./相对路径`，其他位置用 `@绝对路径`（敏感目录会被拒绝）。
2. 资源写法（详见 `docx-xml.md`）：公开网络图片用 `<img href="URL"/>`；已有本地图片用 `<img path="@./downloads/image.png"/>`，
   附件用 `<source path="@./<work_dir>/report.pdf"/>`；画板用 `<whiteboard type="svg" path="@./<work_dir>/diagram.svg"/>`
   （也可把 Mermaid / PlantUML / SVG 源码内联在标签内，复杂画板按 `feishu-cli-visual` 的 board 工作流制作）；
   HTML 组件用 `<html5-block path="@./<work_dir>/widget.html"/>`，HTML 规范见 `docx-xml-extended-blocks.md`。
   `doc create` 与 `doc content-update` 的 XML 写入都会处理这些资源。
3. 首次写入后发现 XML 问题只修最小范围，不无故重写正确内容。

### Step 6：Draft Profile Check

1. 执行 `feishu-cli doc script --command parse --content "@./<draft_path>"`（自动加载保存的决策）。命令成功不代表通过，
   看输出的 `assessment.status`；`failed` 时按 `diagnostics[]` 的 `code` / `suggested` 局部修复后重新解析，直到 `passed`。
   只有草稿为空、截断或结构无效时才全文重写。
2. `passed` 只覆盖已启用的检查：先对照用户要求确认该声明的约束都已写进决策，再按 `docx-xml.md` 复查标签、属性和取值，
   并依据原则检查事实与来源、用户硬约束、所选 contract / adapter 以及 `visual_plan`。

### Step 7：创建文档并处理局部失败

1. 只有最新草稿通过 Step 6 后才创建，并始终使用同一个 `draft_path`：

   ```bash
   feishu-cli doc create --doc-format xml --content-file "./<draft_path>" -o json
   ```

   草稿已含 `<title>`，**不要再传 `--title`**（否则服务端过滤重复标题并返回 warning `degrade_code=1017`）。
   需要放到指定位置时加 `--folder` / `--parent-token`（知识库节点）/ `--parent-position`；身份、授权与 owner 交付见 `../write/workflow.md`。
2. 返回 `warnings`、局部资源失败或回读发现问题时，**不要再次新建文档**：按 `../write/workflow.md` 用
   `doc content-update --doc-format xml` 对已建文档做最小范围修复（失败的图片、附件、画板或遗漏章节都用它补写），再用
   `feishu-cli doc read <document_id> --engine docs_ai --with-ids` 回读确认。
3. 可用 `feishu-cli doc script --command parse --doc <document_id>` 复核线上文档的画像（`--presentation-decision "@./<work_dir>/.presentation-decision.json"` 复用同一基线）。

### Step 8：交付

保留 `work_dir` 及其中的草稿。最终只交付用户需要的结果，说明必要来源、未关闭的缺口、异常或阻塞原因，以及文档链接。

## 相关参考

- [`references/doc-script.md`](references/doc-script.md)：`doc script` 参数、Presentation Decision 字段、诊断码与资源预检。
- [`references/docx-xml.md`](references/docx-xml.md)：DocxXML 写作规范（资源、标题编号、表格、画板、颜色、转义）。
- [`references/docx-xml-extended-blocks.md`](references/docx-xml-extended-blocks.md)：书签、按钮、日期提醒、电子表格、任务、HTML 组件、OKR。
- [`references/docs-ai-markdown.md`](references/docs-ai-markdown.md)：用 Markdown 写入 docs_ai 时的转义与图片规则。
