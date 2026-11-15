# Slides XML（SML 2.0）速查

整理自官方 lark-cli `lark-slides` 技能的 XML 摘要（MIT），只保留生成与编辑页面最常用的部分。
协议命名空间：`https://www.larkoffice.com/sml/2.0`；标准页面 960×540。

## 结构规则

1. `<presentation>` 直接子元素只有 `<title>`、`<theme>`、`<slide>`
2. `<slide>` 直接子元素只有 `<style>`（页面背景）、`<data>`（页面元素）、`<note>`（演讲者备注）
3. 页面元素都放在 `<data>` 里：`shape`、`line`、`polyline`、`img`、`table`、`icon`、`embed`、`chart`
4. 文字只能写在 `<content>` 里：`<content><p>…</p></content>`，不能把文字直接放在 `<shape>` 下
5. 提交给 add-slide / update-slide / create 的是**单个完整 `<slide>`**，不要带 `<?xml ...?>` 声明
6. 特殊字符按 XML 规则转义（`&amp;` `&lt;` `&gt;`）

## 最小页面

```xml
<slide xmlns="https://www.larkoffice.com/sml/2.0">
  <style><fill><fillColor color="rgba(245,245,240,1)"/></fill></style>
  <data>
    <shape type="text" topLeftX="60" topLeftY="40" width="840" height="60">
      <content textType="title" fontSize="32" wrap="true" autoFit="normal-auto-fit"><p>标题</p></content>
    </shape>
    <img src="@./cover.png" topLeftX="60" topLeftY="120" width="400" height="300"/>
  </data>
  <note><content><p>演讲备注</p></content></note>
</slide>
```

## content（文字）

| 属性 | 说明 |
|------|------|
| `textType` | `title` / `headline` / `sub-headline` / `body` / `caption`（不设 `fontSize` 时的兜底字号偏大：54/38/32/16/12） |
| `fontSize` | **显式设置**，不要依赖 textType 兜底 |
| `color` | 文字颜色（用 `color`，不是 `fontColor`） |
| `textAlign` / `verticalAlign` | 对齐 |
| `lineSpacing` | 写 `multiple:1.5` 或 `fixed:24`，不要写裸数字 |
| `wrap="true" autoFit="normal-auto-fit"` | 大字号或长文字必加，自动换行缩排防溢出 |
| `bold` / `italic` / `underline` / `strikethrough` | 内容级样式 |

`<content>` 直接子元素只有 `<p>`、`<ul>`、`<ol>`；`<p>` 内可混排 `<strong>`、`<em>`、`<u>`、`<del>`、`<span>`、`<a href>`、`<br/>`。

```xml
<content textType="body" fontSize="14">
  <p>正文 <strong>加粗</strong> <a href="https://example.com">链接</a></p>
  <ul><li><p>要点 1</p></li><li><p>要点 2</p></li></ul>
</content>
```

## 常用元素

| 元素 | 定位属性 | 要点 |
|------|---------|------|
| `<shape type="...">` | `topLeftX` `topLeftY` `width` `height` | `type`：`text`（文本框）、`rect`、`round-rect`、`ellipse`、`triangle`、`diamond`、`custom`（配 `path`）等；子元素 `<fill>`、`<border>`、`<content>`。`rect` 只是形状不是容器，叠放的文字/图片与它平级 |
| `<line>` | `startX` `startY` `endX` `endY` | 不是 x1/y1/x2/y2；`<border color width>` 控制线 |
| `<polyline>` | `topLeftX` `topLeftY` `width` `height` | 折线/曲线连接，`<border>` 必填 |
| `<img src>` | `topLeftX` `topLeftY` `width` `height` | src 只能是 file_token 或 `@本地路径` 占位符；比例与原图不同会被裁剪；是 `<img>` 不是 `<image>` |
| `<icon iconType>` | 同上 | IconPark 路径，如 `iconpark/Charts/chart-line.svg`，必须 `<fill>` 上色；路径从 [`iconpark.md`](iconpark.md) 常用清单选，不要凭印象拼 |
| `<table>` | `topLeftX` `topLeftY` `width` `height` | 子元素 `<colgroup><col width/></colgroup>` 与 `<tr height><td>…</td></tr>`；`<td>` 只放 `<fill>`、`<content>`；表头加背景色 |
| `<chart>` | 同上 | 必须有 `<chartPlotArea>` 与 `<chartData>`；见下方「图表」一节，照 [`slides_chart_demo.xml`](slides_chart_demo.xml) 改 |
| `<embed>` | 同上 | 内含一个标准 `<svg xmlns="http://www.w3.org/2000/svg">` |

## 颜色与背景

```xml
<fill><fillColor color="rgba(51,112,255,1)"/></fill>
<border color="rgba(43,47,54,1)" width="2"/>
<fillColor color="linear-gradient(135deg,rgba(30,60,114,1) 0%,rgba(59,130,246,1) 100%)"/>
```

渐变必须用 `rgba()` 且带百分比停靠点，否则服务端回退为白色。页面背景写在 `<slide><style><fill>…</fill></style>`。

## 编辑时的 id

- `slides get` 返回的 XML 里每个元素带 `id`（如 `bkW`），页面带 `<slide id="...">`
- `replace-slide` 的 `block_id` 用元素 id；`update-slide` 写回时保留元素 id 表示"原地更新"，删掉 id 表示新插入
- `--remove-attr-id` 读出来的 XML 没有 id，只适合浏览，不能用来编辑

## 自查清单

- 所有元素在 960×540 内，主体内容离边缘 ≥ 40px
- 每个 `<shape>` 都有 `<content>`（replace-slide 会自动补空的，但带文字时要自己写全）
- 字号显式设置，长文字加 `wrap="true" autoFit="normal-auto-fit"`
- 图片用 `@` 占位符或当前演示文稿的 file_token
- 写回前可 `slides screenshot --content @page.xml` 先看渲染效果

## 图表 `<chart>`

- 完整示例：[`slides_chart_demo.xml`](slides_chart_demo.xml)（7 页，覆盖 column / bar / line / area / pie / radar / combo，
  含堆叠、平滑、双轴、数据标签、配色主题），复制其中一个 `<chart>` 改 `chartData` 最快。
- 元素与属性定义：[`slides_chart_schema.xml`](slides_chart_schema.xml)（官方 XSD 的图表节选，按需搜索元素名，不必通读）。
- `<chartPlot type>` 取值：`pie` `column` `bar` `line` `area` `radar` `combo` `bubble` `waterfall` `pareto`。
  `bubble` / `waterfall` / `pareto` 不在 demo 里，配置分别写在 `<chartExtra>` 的 `<chartBubble>` / `<chartWaterfall>` /
  `<chartPareto>` 下（定义见 schema 节选的 `ChartBubbleType` / `ChartWaterfallType` / `ChartParetoType`）。
- 数据模型：`<chartData>` 下 `<dim1>` 是分类（一个 `<chartField>`，逗号分隔），`<dim2>` 是数值系列（每个系列一个 `<chartField>`），
  dim1 第 i 个值对应每个系列的第 i 个值。

瀑布图（实测通过服务端 lint 并正确渲染）：数值是**增减量**；`firstValueAsTotal="true"` 把首值画成起点总量柱，
`lastValueAsSubtotal="true"` 在末尾自动追加一根「小计」柱（不要再手写期末值，否则会被当成又一次增量）。

```xml
<chart topLeftX="60" topLeftY="100" width="840" height="400">
  <chartPlotArea>
    <chartPlot type="waterfall">
      <chartExtra>
        <chartWaterfall firstValueAsTotal="true" lastValueAsSubtotal="true"/>
      </chartExtra>
    </chartPlot>
    <chartAxes>
      <chartAxis type="x"><chartLabel fontSize="10"/></chartAxis>
      <chartAxis type="y" position="left"><chartGridLine color="rgb(226, 232, 240)"/><chartLabel fontSize="10"/></chartAxis>
    </chartAxes>
  </chartPlotArea>
  <chartData>
    <dim1><chartField name="项目">期初,收入增长,成本上升,汇率影响</chartField></dim1>
    <dim2><chartField name="利润">100,40,-25,-5</chartField></dim2>
  </chartData>
  <chartTitle fontSize="12" bold="true">利润变化（万元）</chartTitle>
</chart>
```
