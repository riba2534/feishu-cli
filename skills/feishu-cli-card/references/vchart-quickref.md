# VChart 图表速查（chart 组件）

> `chart` 组件的 `chart_spec` 字段遵循 **VChart** 的图表定义规范。
> 官方：`https://www.visactor.io/vchart`
> 本文给出 6 种最常用图表在飞书卡片里的可用骨架，直接复制改字段。
>
> **先选形式再画**：柱还是线、要不要干脆一个大数字（column_set + fields），
> 按数据任务判断，见 `feishu-cli-dataviz` 技能（任务→形式启发式 + 反模式清单）。

## 通用外壳

```json
{
  "tag": "chart",
  "element_id": "chart_1",
  "aspect_ratio": "16:9",         // "1:1" | "2:1" | "4:3" | "16:9"  (PC默认16:9/移动端1:1)
  "height": "auto",                // "auto" 或 [100,999]px
  "color_theme": "brand",          // "brand" | "rainbow" | "complementary" | "monochromatic"
  "preview": true,                 // PC 端点击放大查看
  "chart_spec": { /* 下面各图表的 type + data */ }
}
```

**约束**：
- 单卡片 **≤ 5 个 chart**（性能）
- 客户端 **7.1+** 才支持；7.10+ 才支持 `height` 属性
- 移动端不支持：纹理填充、conical 渐变、grid 布局词云

**系列配色（统一色板注入）**：在 `chart_spec` 顶层加 `color` 数组，取
`feishu-cli-dataviz` 统一色板 light 列全 8 色（VChart 按系列顺序从头消费，
写满即可，多余的不生效）；完整可复制示例：

```json
{
  "type": "bar",
  "color": ["#3370ff", "#d99904", "#04b49c", "#ed6d0c", "#7f3bf5", "#f54a45", "#f14ba9", "#2ea121"],
  "data": { "values": [ { "月份": "1月", "渠道": "A", "金额": 100 } ] },
  "xField": "月份",
  "yField": "金额",
  "seriesField": "渠道"
}
```

该 8 色已通过色盲区分度校验（多系列**永远按此固定顺序**；单系列可改取 header
同家族色做强调，规则见 `design.md` §1.1.1）。注意 slot 2/3（`#d99904`/`#04b49c`）
对白底不足 3:1，用到时开 `"label": {"visible": true}` 补数值标签。若目标客户端
版本不消费 `color` 数组（渲染仍是默认主题色），退回 `color_theme` 并按下方选择表取值。

---

## 1. 柱状图（bar）

```json
{
  "type": "bar",
  "title": { "text": "本周各渠道交易额" },
  "data": {
    "values": [
      { "渠道": "A", "金额": 12000 },
      { "渠道": "B", "金额": 8500 },
      { "渠道": "C", "金额": 15200 }
    ]
  },
  "xField": "渠道",
  "yField": "金额",
  "label": { "visible": true },
  "legends": { "visible": false }
}
```

**常用变体**：
- 横向柱状图：加 `"direction": "horizontal"` 并互换 xField/yField
- 分组柱状：`"xField": ["月份", "类别"]`，数据多一列 `类别`
- 堆叠柱状：分组柱 + `"stack": true`

## 2. 饼图 / 环形图（pie）

```json
{
  "type": "pie",
  "data": {
    "values": [
      { "分类": "软件工程", "占比": 40 },
      { "分类": "视觉理解", "占比": 25 },
      { "分类": "Agent",    "占比": 20 },
      { "分类": "长上下文", "占比": 15 }
    ]
  },
  "categoryField": "分类",
  "valueField": "占比",
  "label": { "visible": true, "formatMethod": "{text}: {value}%" },
  "legends": { "visible": true, "orient": "bottom" }
}
```

**环形图**：加 `"innerRadius": 0.6`（内半径比例）。

## 3. 折线图（line）

```json
{
  "type": "line",
  "title": { "text": "7 日访问量趋势" },
  "data": {
    "values": [
      { "日期": "04-11", "UV": 320 },
      { "日期": "04-12", "UV": 412 },
      { "日期": "04-13", "UV": 389 },
      { "日期": "04-14", "UV": 512 },
      { "日期": "04-15", "UV": 478 },
      { "日期": "04-16", "UV": 623 },
      { "日期": "04-17", "UV": 701 }
    ]
  },
  "xField": "日期",
  "yField": "UV",
  "point": { "visible": true },        // 节点圆点
  "smooth": true,                       // 平滑曲线
  "label": { "visible": false }
}
```

**多系列折线**：加 `"seriesField": "系列名"`，数据里多一列分组字段。

## 4. 面积图（area）

```json
{
  "type": "area",
  "data": {
    "values": [
      { "月份": "1月", "收入": 100 },
      { "月份": "2月", "收入": 150 },
      { "月份": "3月", "收入": 180 }
    ]
  },
  "xField": "月份",
  "yField": "收入",
  "area": { "style": { "fillOpacity": 0.3 } }
}
```

## 5. 雷达图（radar）

```json
{
  "type": "radar",
  "data": {
    "values": [
      { "维度": "性能",   "得分": 90, "模型": "4.7" },
      { "维度": "安全",   "得分": 95, "模型": "4.7" },
      { "维度": "推理",   "得分": 92, "模型": "4.7" },
      { "维度": "视觉",   "得分": 98, "模型": "4.7" },
      { "维度": "代码",   "得分": 96, "模型": "4.7" },
      { "维度": "性能",   "得分": 85, "模型": "4.6" },
      { "维度": "安全",   "得分": 92, "模型": "4.6" },
      { "维度": "推理",   "得分": 88, "模型": "4.6" },
      { "维度": "视觉",   "得分": 54, "模型": "4.6" },
      { "维度": "代码",   "得分": 80, "模型": "4.6" }
    ]
  },
  "categoryField": "维度",
  "valueField": "得分",
  "seriesField": "模型",
  "legends": { "visible": true }
}
```

## 6. 仪表盘（gauge）

```json
{
  "type": "gauge",
  "data": {
    "values": [{ "value": 78 }]
  },
  "categoryField": "type",
  "valueField": "value",
  "min": 0,
  "max": 100,
  "title": { "text": "完成度" }
}
```

## 7. 词云（wordCloud）

```json
{
  "type": "wordCloud",
  "data": {
    "values": [
      { "word": "AI", "freq": 100 },
      { "word": "Claude", "freq": 80 },
      { "word": "Opus", "freq": 60 },
      { "word": "Agent", "freq": 50 }
    ]
  },
  "nameField": "word",
  "valueField": "freq",
  "wordCloudConfig": { "layoutMode": "default" }
}
```

**移动端限制**：`layoutMode: "grid"` 在移动端加载失败，用 `default`。

## 8. 漏斗图（funnel）

漏斗阶段是**有序档位**，配色用单色蓝 ramp（浅→深随转化推进）而非多色系列色：
`"color": ["#8ab1ff", "#5c90ff", "#3e72e5", "#2b57bd", "#1f418c"]`。

```json
{
  "type": "funnel",
  "data": {
    "values": [
      { "阶段": "访问", "人数": 10000 },
      { "阶段": "浏览", "人数": 5000 },
      { "阶段": "加购", "人数": 2000 },
      { "阶段": "下单", "人数": 800 },
      { "阶段": "支付", "人数": 600 }
    ]
  },
  "categoryField": "阶段",
  "valueField": "人数",
  "label": { "visible": true }
}
```

## 9. 组合图（combination）

柱状 + 折线叠加，**两个系列必须同量纲共一个轴**（例如营收柱 + 成本折线）：

```json
{
  "type": "common",
  "data": {
    "id": "id_0",
    "values": [
      { "月份": "1月", "营收": 100, "成本": 62 },
      { "月份": "2月", "营收": 150, "成本": 88 },
      { "月份": "3月", "营收": 180, "成本": 95 }
    ]
  },
  "series": [
    { "type": "bar",  "xField": "月份", "yField": "营收" },
    { "type": "line", "xField": "月份", "yField": "成本" }
  ]
}
```

⚠ 量纲不同的两个指标（金额 vs 百分比）**不要**塞进一张组合图 —— 双刻度的对齐是任意的，
会凭空发明相关性。拆成两个 chart 组件上下排，或把两者指数化到同一基期。

---

## color_theme 选择（未注入 color 数组时的回退）

| color_theme | 适用 |
|------------|------|
| `brand` | 默认（飞书主题色）；**优先注入统一色板 color 数组，回退时才靠它** |
| `rainbow` | 多分类（饼图、分组柱）的回退选项；有 color 数组时不需要 |
| `complementary` | 对比两组数据 |
| `monochromatic` | 单色渐变 —— 量级/有序档位场景（热力、漏斗）的正确语义 |

## label / legends 常见配置

```json
{
  "label": {
    "visible": true,
    "position": "top",              // top | inside | bottom
    "style": { "fontSize": 12 },
    "formatMethod": "{value}%"      // 自定义格式化
  },
  "legends": {
    "visible": true,
    "orient": "bottom",             // top | right | bottom | left
    "position": "start"
  }
}
```

## 坑点速查

| 坑 | 解决 |
|----|------|
| 图表显示一片空白 | 查 `xField`/`yField` 是否和 `data.values` 的 key 匹配 |
| 移动端渲染失败 | 移除 `texture` / `conical gradient` / `wordCloud.grid` 等移动端不支持项 |
| 低版本客户端不显示 | 客户端 < 7.1 展示"请升级客户端" |
| label 不显示 | 加 `"label": { "visible": true }` |
| 饼图分类标签在图外 | `"label": { "visible": true, "position": "outside" }` |
| 数据太多图表拥挤 | 减数据点数 / 调 `aspect_ratio: "2:1"` 拉宽 |
