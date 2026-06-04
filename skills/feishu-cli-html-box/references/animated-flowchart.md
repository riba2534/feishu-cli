# Recipe: Animated Flowchart

把结构化的图数据（节点 / 连线 / 时间线）生成一份**自包含的单文件动画 HTML**，再用本技能的 `publish_html_box.py` 嵌入飞书文档。适合流程图、架构图、拓扑图、Agent 编排 / 多 Agent 协作、时序 / 数据流的「会动」可视化。

## 产出物

零依赖单文件 HTML：

- 从 `nodes` 和 `edges` 渲染的 SVG 拓扑。
- 自动播放的时间线：节点高亮、连线高亮、消息 token 流动、字幕、进度点。
- 极简控件：上一步、播放 / 暂停、下一步、可点击的进度点。
- 默认固定浅色主题，适配飞书文档。
- iframe 内无竖向滚动条（内滚动会干扰文档滚动）。
- 响应式全屏：飞书 HTML Box 全屏打开时拓扑会放大。

默认**不用** React、Framer Motion、Mermaid 运行时、外部 CDN、倍速控件、重播按钮、纯键盘控件，也不用内部可滚动 iframe。

## 工作流

### 1. 准备 pattern.json

按 [pattern-schema.md](pattern-schema.md) 组织一个 JSON，参考 [../examples/supervisor.json](../examples/supervisor.json)。

### 2. 生成 HTML

```bash
python3 skills/feishu-cli-html-box/scripts/animate_diagram.py \
  --pattern pattern.json \
  --out animated-diagram.html
```

### 3. 本地预览（可选）

```bash
python3 -m http.server 8799 --bind 127.0.0.1
```

打开 `http://127.0.0.1:8799/animated-diagram.html`，确认：

- 自动播放走完所有时间线步骤。
- 播放 / 暂停、上一步 / 下一步、进度点都可用。
- 背景是浅色，不是黑色。
- iframe 内容无竖向滚动条。
- 全屏预览用更大的视口，而不是卡在小卡片宽度。
- token 流动方向正确；`!edgeId` 表示反向流动。

### 4. 嵌入飞书文档

新建文档：

```bash
python3 skills/feishu-cli-html-box/scripts/publish_html_box.py \
  --html animated-diagram.html \
  --title "Animated Diagram"
```

嵌入已有文档：

```bash
python3 skills/feishu-cli-html-box/scripts/publish_html_box.py \
  --html animated-diagram.html \
  --doc-token <docx_token>
```

发布机制与铁律见上级 [SKILL.md](../SKILL.md)（第 3、6、8 节）。

## Pattern 数据规则

- 画布坐标基于 `viewBox="0 0 900 540"`。
- 节点需要稳定的 `id`、`x`、`y`、`label`；`w`、`h`、`sub`、`kind` 可选。
- 连线以 edge ID 为 key。时间线的 `fire` 用这些 ID 引用连线。
- 时间线 `fire` 项前缀 `!` 表示 token 反向流动。
- 字幕保持简短；允许内联 `<b>` 和 `<code>`。
- `kind: "accent"` 用于当前编排者，`kind: "dark"` 用于最终 / 输出节点，`kind: "user"` 用于用户气泡。
