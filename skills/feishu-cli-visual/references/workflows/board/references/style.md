# 配色系统

> **色值唯一出处**：`skills/feishu-cli-visual/references/workflows/dataviz/references/palette.md`（统一色板，
> 已通过色盲区分度/对比度脚本校验）。本文件只讲画板场景的**取用方式**与主题变体，
> 不自创色值。改色后用
> `skills/feishu-cli-visual/references/workflows/dataviz/scripts/validate_palette.js` 复验。

## 怎么上色（最重要）

上色步骤：

1. **找出图中有几个分组**（层级、分支、类别、阶段...）
2. **为每个分组按序取一个家族**（下表从上往下取，不跳、不循环）
3. **分组背景**用该家族淡底色 + 低透明度 -- 告诉读者"这块是一个整体"
4. **分组内节点**用白色填充 + 该家族深边色 border_color -- 告诉读者"这些属于这个分组"

分组映射（默认主题，浅底/深边派生对）：

| 分组 | 背景 fill_color | 背景/节点 border_color |
|------|----------------|------------------------|
| 第 1 组 | #E6EFFF（蓝） | #1446C2 |
| 第 2 组 | #FDECD1（琥珀） | #714E00 |
| 第 3 组 | #D3F8EF（青） | #006456 |
| 第 4 组 | #FFEADF（橙） | #893B00 |
| 第 5 组 | #EFECFF（紫） | #611DC5 |
| 第 6 组 | #FFE9E6（红） | #A30011 |
| 第 7 组 | #FFE7F1（品红） | #990064 |
| 第 8 组 | #E0F6DD（绿） | #0C6800 |
| 内部节点 | #FFFFFF | 跟随所属分组 |

超过 8 个分组不生成第 9 个颜色 —— 合并相近分组或拆图。

**各类图表怎么上色**：
- 架构图有 3 层 -- 每层一个家族，层背景淡底填充（opacity <= 25），层内节点白色 + 深边框
- 对比表有 3 列 -- 每列表头一个家族，该列数据单元格用同家族边框
- 组织架构有 4 个部门 -- 每个部门一个家族，子部门白色 + 同家族边框
- 流程图 -- 起止节点一个家族，判断节点一个家族，步骤节点白色

> 用户配色优先。用户指定了色值/风格时以用户为准。用户只给 1-2 个色值时，
> 推导完整色板（主色 -> 淡底 -> 深边框 -> 灰调连线色），推导结果先跑
> `validate_palette.js` 再用。

## 数据图表的系列色（柱/线/饼落画板时）

结构图用上面的"浅底/深边"；**数据系列**（柱、折线、饼片、散点）用 categorical
原色，按固定顺序截取所需个数：

```
#3370FF, #D99904, #04B49C, #ED6D0C, #7F3BF5, #F54A45, #F14BA9, #2EA121
```

**原样按序取用时无需校验**（该色板已预校验，结论见 `feishu-cli-visual` 的 palette.md）。
仅当改色值/换底色时先跑校验（先定位再调用，任意 CWD 可用）：

```bash
VP=~/.claude/skills/feishu-cli-visual/references/workflows/dataviz/scripts/validate_palette.js
[ -f "$VP" ] || VP=skills/feishu-cli-visual/references/workflows/dataviz/scripts/validate_palette.js   # 仓库内开发时（CWD=仓库根）
node "$VP" "#3370FF,#D99904,#04B49C" --mode light
```

散点/气泡类（任意两色可相邻）加 `--pairs all`，且系列色改用 all-pairs 安全子集
`#3370FF / #D99904 / #04B49C / #F54A45`（≤4 系列；更多系列拆小倍数或复合编码）。

画板图**渲染后没有 tooltip 兜底**，所以：关键数值直接标签必须充分（端点/极值/
每根柱的值择要标注）；文字一律用 ink 色（#1F2329），不穿系列色 —— 系列身份由
文字旁的色块/图例承载。形式选择与反模式清单见 `feishu-cli-visual` 技能。

---

## 结构规则

### 分组 -- 不同层/分组必须用不同颜色

按序取 2-4 个家族，每个代表一个分组。同组节点视觉完全一致（fill_color、border_color 相同）。

### 分层 -- 外重内轻

- 外层（大分区背景）：淡底填充，fill_opacity <= 25
- 内层（具体节点）：白色填充（opacity=100） + 分组色边框

### 清晰

- 所有节点有边框（border_style=solid, border_width=medium）
- 间距不粘连（同层 >= 30px，有连线 >= 60px）
- 文字清晰可读（font_size >= 14）
- 连线用主题连线灰（默认 #BBBFC4，科技暗色 #4A4A4A），不抢节点注意力

### 统一参数

| 参数 | 值 | 为什么 |
|------|---|--------|
| border_width | `medium` | 让边框清晰可见 |
| border_style | `solid` | 统一的边框风格 |
| fill_opacity（节点） | 100 | 实心填充 |
| fill_opacity（背景） | <= 25 | 不遮挡上层 |
| font_size（正文） | >= 14 | 可读 |
| font_size（标题） | >= 24 | 醒目 |
| font_size（辅助） | >= 13 | 不费眼 |

---

## 主题变体

主题 = 同一色相体系上的取用组合，换气质不换体系。未指定时用**默认**。

| 主题 | 适用场景 | 取用方式 |
|------|---------|---------|
| 默认（多色分组） | 通用图表、说明文档 | 上表 8 组浅底/深边对按序取用 |
| 商务（蓝单色相） | 汇报、企业架构、正式文档 | 背景用 sequential 浅步 #CEDFFF / #B7D0FF / #A0C0FF，边框 #2B57BD，强调/表头 #1C3672 反白 |
| 科技（暗色） | 技术架构、DevOps、监控 | 画布 #1F1F1F，节点 fill #2A2A2A，边框按序取 dark 系列色 #3370FF / #BF8600 / #00AD96 / #EA6A03，文字 #EBEBEB，连线 #4A4A4A |
| 清新（青绿单色相） | 流程图、用户旅程、教程 | 青/绿两家族派生对 #D3F8EF/#006456、#E0F6DD/#0C6800 交替，强调 #0C6800 反白 |
| 极简（无彩） | 论文配图、学术报告 | 中性阶：背景 #F2F3F5，边框 #DEE0E3 / #8F959E，文字 #1F2329，强调 #1F2329 反白 |

一张画板只用一个主题，不混搭。**商务/清新/极简是低色数主题**（可区分档位分别约
3 / 2 / 3 档）：分组数超过档位数时改用默认主题，不为凑数自创新色。连线灰随主题
（默认 #BBBFC4，科技暗色 #4A4A4A）。

---

## 各元素怎么画

> 以下示例使用默认主题第 1 组（蓝）。换主题/分组时替换对应颜色，结构不变。

### 图表标题

大号深色文字，居中。用 composite_shape + 无边框无填充模拟纯文本。

```json
{"style": {"fill_opacity": 0, "border_style": "none"},
 "text": {"font_size": 24, "font_weight": "bold", "horizontal_align": "center"}}
```

### 分区背景

淡底填充 + 低透明度 + 对应深边框。内部放白色节点。

```json
{"style": {"fill_color": "#E6EFFF", "fill_opacity": 25,
           "border_style": "solid", "border_color": "#1446C2", "border_width": "narrow", "border_opacity": 40},
 "z_index": 0}
```

### 分区标签

独立文本节点，深色文字。通过 composite_shape 模拟。

```json
{"style": {"fill_opacity": 0, "border_style": "none"},
 "text": {"font_size": 18, "font_weight": "bold", "horizontal_align": "left"}}
```

### 内容节点

白色填充，边框颜色跟随所属分组。

```json
{"style": {"fill_color": "#FFFFFF", "fill_opacity": 100,
           "border_style": "solid", "border_color": "#1446C2", "border_width": "medium", "border_opacity": 100},
 "z_index": 10}
```

白色节点的 border_color 取决于所属分组：
```
第 1 组（蓝）: border_color="#1446C2"
第 2 组（琥珀）: border_color="#714E00"
第 3 组（青）: border_color="#006456"
独立节点: border_color="#DEE0E3"
```

### 表头

深色填充 + 白色文字。

```json
{"style": {"fill_color": "#1F2329", "fill_opacity": 100,
           "border_style": "solid", "border_color": "#1F2329", "border_width": "medium", "border_opacity": 100},
 "text": {"font_size": 15, "font_weight": "bold", "horizontal_align": "center"},
 "z_index": 10}
```

### 连线

统一用灰色，不用彩色。

```json
{"style": {"border_color": "#BBBFC4", "border_opacity": 100,
           "border_style": "solid", "border_width": "narrow"},
 "z_index": 50}
```

### 辅助说明

灰色小字，弱化不抢注意力。

```json
{"text": {"font_size": 13, "font_weight": "regular"},
 "style": {"fill_opacity": 0, "border_style": "none"}}
```

---

## 常见错误

错误：每个节点一种颜色 -> 读者分不清分组
正确：同组节点视觉一致（相同 fill_color + border_color）

错误：内外层都用重色 -> 读者不知道先看哪里
正确：外层淡底（opacity <= 25），内层白色实心

错误：连线用和节点一样的彩色 -> 抢注意力
正确：连线统一用主题连线灰（默认 #BBBFC4，科技暗色 #4A4A4A）

错误：节点没边框 -> 和背景融为一体
正确：所有节点 border_style=solid, border_width=medium

错误：全图黑白灰，没有颜色区分 -> 无法识别分组
正确：不同分组按序取不同家族

错误：自创色值 / 凭感觉调色 -> 色盲可分性与对比度无保障
正确：色值一律取自统一色板，改动后跑 validate_palette.js

错误：数据系列用浅底色 -> 柱/线太浅读不出
正确：数据系列用 categorical 原色，结构分组才用浅底/深边对
