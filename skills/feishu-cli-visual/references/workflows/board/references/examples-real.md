# 14 张实战画板档案

这是 2026-05 实战的 14 张画板档案，覆盖图表 / 跨领域设计 / 极限挑战三类场景。原始作品由 Claude Opus 4.7 自由 SVG 设计后，通过本 skill 的 5 步管道翻译为飞书原生节点。

每个条目记录：示例画板 token / 节点数 / 视觉设计模式 / SVG 元素组合 / 推荐工作路径。**Claude 用这份档案做设计参考**——拿到"画个增长飞轮"的需求，能直接看到对应模式应用哪些 SVG 元素、节点规模大概多少。

---

## 目录

- [基本图表](#第一弹基本图表6-张)
- [跨领域作品](#第二弹跨领域作品5-张)
- [极限挑战](#第三弹极限挑战3-张)
- [设计模式索引](#设计模式索引按视觉模式查找)
- [节点密度排行](#节点密度排行按规模)

## 第一弹：基本图表（6 张）

### 1. 增长飞轮 · Growth Flywheel

- **示例 token**: `board_token_xxx`
- **节点数**: 48（原文档）/ 30（复刻）
- **类型分布**: text_shape:24 / svg:4（扇形）/ composite_shape:2 / connector:0
- **设计模式**: 圆环切割 + 极坐标外围标签
- **SVG 元素组合**:
  - `<circle>` 嵌套圆环（外圈 R=300 / 中圈 R=200 / 内圈 R=80）
  - `<path>` 4 段扇形（每段 90°）
  - `<text>` 外围 4 个阶段标签（顶/右/下/左）
  - 中心 `<text>` "SaaS Flywheel"
- **关键技术点**: 极坐标公式 `(cx + r·cosθ, cy + r·sinθ)` 均匀分布 N 个标签
- **推荐路径**: C（SVG → 原生节点）

### 2. 鱼骨分析 · Fishbone Diagram

- **示例 token**: `board_token_xxx`
- **节点数**: 71（原文档）/ 67（复刻）
- **类型分布**: text_shape:30 / connector:28 / composite_shape:8 / svg:1
- **设计模式**: 对称斜骨 + 同色系分组
- **SVG 元素组合**:
  - 主骨 `<line>`（水平居中）
  - 5-6 个维度的斜骨 `<line>`（上下交替）
  - 每个斜骨下挂 3-4 根 `<line>` 小骨
  - 分类标签卡 `<rect>` + `<text>`
- **关键技术点**: 三角函数计算等距挂载点（沿斜骨参数化 t = i / (n+1)）
- **推荐路径**: C

### 3. 价值金字塔 · Value Pyramid

- **示例 token**: `board_token_xxx`
- **节点数**: 56（原文档）/ 28（复刻）
- **类型分布**: text_shape:21 / svg:5 / composite_shape:1 / connector:1
- **设计模式**: 层级递减 + 描述外置
- **SVG 元素组合**:
  - 4 层 `<polygon>` 等差递减宽度（顶 80 → 220 → 380 → 560 → 760 底）
  - 中心 `<text>` 每层标题
  - 右侧 `<text>` 每层描述 + 示例
  - 左侧 `<line>` "价值密度"轴 + 箭头
- **关键技术点**: 渐变色（冷色 → 暖色 暗示价值密度递增）
- **推荐路径**: C

### 4. 流量归因桑基图 · Traffic Attribution Sankey

- **示例 token**: `board_token_xxx`
- **节点数**: 97（原文档）/ 85（复刻）
- **类型分布**: svg:35（流条）/ text_shape:33 / composite_shape:17
- **设计模式**: 三层流动 + cubic-bezier 流条
- **SVG 元素组合**:
  - 3 列节点 `<rect>`（来源 / 落地页 / 行为）
  - 多条 `<path d="M ... C ...">` cubic-bezier 流条
  - 流条宽度精确等于流量值（高度按 UV 量等比例缩放）
- **关键技术点**: bezier 控制点 `cx = (x1+x2)/2`，按对端 y 排序减少交叉
- **推荐路径**: C
- **替代原图**: 这张是 2026-04 更新后替换原"转化漏斗"的

### 5. 产品路线图 · Product Roadmap

- **示例 token**: `board_token_xxx`
- **节点数**: 96（原文档）/ 93（复刻）
- **类型分布**: text_shape:54 / composite_shape:28 / connector:14 / svg:1
- **设计模式**: 横向时间轴 + 上下交替里程碑卡
- **SVG 元素组合**:
  - 主轴 `<line>`（水平贯穿） + 箭头 `<polygon>`
  - 等距 `<circle>` 圆点（每个里程碑）
  - `<line>` 虚线引导（圆点 → 卡片）
  - `<rect>` 卡片 + `<text>` 标题 / 副标题 / 描述
- **关键技术点**: 上下交替放置避免视觉拥挤；按年份 `<line>` 刻度
- **推荐路径**: C

### 6. SaaS 仪表盘 UI · Analytics Dashboard

- **示例 token**: `board_token_xxx`
- **节点数**: 187（原文档）/ 171（复刻）
- **类型分布**: text_shape:104 / composite_shape:62 / connector:16 / svg:5
- **设计模式**: UI Mockup 嵌套
- **SVG 元素组合**:
  - 深色侧边栏 `<rect>` + 导航项 `<text>`
  - 顶部 nav `<rect>` + 搜索框 + 头像 `<circle>`
  - 4 个 KPI 卡片 `<rect>` + 大字 `<text>`（数值）+ 小迷你 `<rect>` 柱状图装饰
  - 折线图 `<polyline>`（实际 + 虚线预测） + `<circle>` 数据点
  - 环形图 `<path>` 扇形分割 + 内圆 `<circle>` 遮盖
  - 排行榜表格 `<line>` 分隔 + `<text>` 数据
- **关键技术点**: 圆角 `<rect rx="8">` 模拟卡片质感；同色系不同 opacity 区分主次
- **推荐路径**: C

---

## 第二弹：跨领域作品（5 张）

### 7. 房屋平面图 · Floor Plan

- **示例 token**: `board_token_xxx`
- **节点数**: 288（原文档）/ 119（复刻）
- **类型分布**: composite_shape:116 / text_shape:106 / connector:50 / svg:16
- **设计模式**: 平面建筑布局 + 家具示意
- **SVG 元素组合**:
  - 嵌套 `<rect>` 表达墙体厚度（外墙 12px / 内墙 6px）
  - 房间分区 `<rect>` 不同色填充（客厅 / 卧室 / 厨房 / 卫浴 / 玄关）
  - 家具 `<rect>` / `<circle>`（沙发 / 床 / 餐桌 / 灶台 / 马桶）
  - 门 `<path>` 弧线表达开启范围
  - 窗户 `<rect>` 蓝色低透明度
  - 尺寸标注 `<line>` + 端点 + `<text>` 数值
  - 指北针 `<circle>` + `<polygon>` 指针
- **关键技术点**: 用透明度区分功能区，弧线模拟门开启
- **推荐路径**: C

### 8. 宣传海报 · AI Builder Summit 2026

- **示例 token**: `board_token_xxx`
- **节点数**: 153（原文档）/ 124（复刻）
- **类型分布**: composite_shape:86 / text_shape:56 / svg:4 / group:3 / connector:4
- **设计模式**: 斜切几何 + 大字标题 + QR 区域
- **SVG 元素组合**:
  - 渐变背景（深紫 → 品红 linearGradient）
  - 顶部斜切 `<polygon>` 色块
  - 巨大 `<text>` 主标题（98px font-weight 900）
  - 几何装饰 `<circle>` + `<polygon>`
  - 嘉宾矩阵：6 个 `<circle>` 头像 + `<text>` 姓名 / 公司
  - 主题标签：5 个 `<rect rx>` 圆角胶囊
  - QR 区域：10×10 网格 `<rect>` 模拟二维码图案
- **关键技术点**: 强对比色块；CSS-like font-weight 数字字号；网格生成 QR 图案
- **推荐路径**: C

### 9. Mobile App UI · 健康追踪

- **示例 token**: `board_token_xxx`
- **节点数**: 157（原文档）/ 67（复刻）
- **类型分布**: text_shape:41 / composite_shape:24 / connector:2
- **设计模式**: iOS 风格 UI Mockup
- **SVG 元素组合**:
  - iPhone Pro 比例 viewBox（430×900）
  - 状态栏 `<text>` 时间 + 电量
  - 心率主卡：`<rect rx="20">` + 大字数值 + `<polyline>` 心电波形
  - 3 个圆形进度环：`<circle>` 背景 + `<circle stroke-dasharray>` 进度
  - 睡眠堆叠条：4 段不同色 `<rect>` 横向拼接
  - 周趋势柱状：等距 `<rect>` + 文字
  - 底部 tab bar：4 个 `<text>` icon + 标签
- **关键技术点**: stroke-dasharray 控制圆环弧长 = 进度
- **推荐路径**: C

### 10. 地铁线路图 · 云栖城

- **示例 token**: `board_token_xxx`
- **节点数**: 137（原文档）/ 117（复刻）
- **类型分布**: composite_shape:66 / text_shape:46 / connector:18 / group:5 / svg:2
- **设计模式**: 多色线路网 + 站点标注
- **SVG 元素组合**:
  - 5 条 `<polyline>` 线路（红 1 号 / 蓝 2 号 / 绿 3 号 / 紫 4 号环线 / 橙 5 号）
  - 普通站点：`<circle>` 白底彩边
  - 换乘站：`<rect rx>` 圆角矩形 + 换乘线号
  - 终点站：`<text>` 方向箭头
  - 图例区 `<rect>` + 线段示例 `<line>` + 站数统计
  - 比例尺 `<line>` + 刻度
  - 指北针 `<polygon>`
- **关键技术点**: 仅水平 / 垂直 / 45° 折角（伦敦地铁图风格）
- **推荐路径**: C

### 11. 插画 · 雨后山林清晨

- **示例 token**: `board_token_xxx`
- **节点数**: 107（原文档）/ 101（复刻）
- **类型分布**: composite_shape:45 / connector:26 / svg:25（山脉 / 太阳光晕）/ group:5 / text_shape:1
- **设计模式**: 多层视差 + 暖色调
- **SVG 元素组合**:
  - 渐变天空 `<linearGradient>`（晨曦橘 → 蓝紫）
  - 多层山脉 `<polygon>`（远山雾色 / 中山蓝灰 / 近山深蓝）
  - 太阳光晕：3 层 `<circle>` 同心圆 不同 opacity
  - 12 棵简化树：`<polygon>` 三角形 + `<rect>` 树干
  - 飞鸟 `<path>` V 形
  - 雾气 `<linearGradient>` 半透明覆盖
  - 雨滴 `<line>` 细短线
  - 水面反光 `<rect>` 横向小色块
- **关键技术点**: 渐变叠加营造氛围；polygon 山脉做出层次感
- **推荐路径**: C

---

## 第三弹：极限挑战（3 张）

### 12. 化学元素周期表 · Periodic Table

- **示例 token**: `board_token_xxx`
- **节点数**: 659（原文档）/ 647（复刻）
- **类型分布**: text_shape:525 / composite_shape:133 / connector:1
- **设计模式**: 密集网格 + 类别分色
- **SVG 元素组合**:
  - 118 个 `<rect>` 元素方格（按 IUPAC 7 周期 × 18 族布局）
  - 每方格 4 个 `<text>`（原子序数 / 元素符号 / 中文名 / 原子质量）
  - 镧锕系单独在底部两行
  - 10 类别配色（碱金属 / 卤素 / 惰性气体 / 过渡金属 / 类金属 / 镧 / 锕 等）
  - 类别图例 `<rect>` + `<text>`
  - 族 / 周期标号
- **关键技术点**: Python 循环按 IUPAC 表程序化生成；精准毫米级排布
- **推荐路径**: C（仅 Python 程序化生成可行，手写不现实）
- **节点数解释**: 118 × 4 text + 118 rect + 类别图例 + 标号 ≈ 647

### 13. 机械时钟内部 · Movement Anatomy

- **示例 token**: `board_token_xxx`
- **节点数**: 168（原文档）/ 316（复刻）
- **类型分布**: composite_shape:259 / connector:43 / text_shape:13 / svg:1
- **设计模式**: 机械结构解剖图
- **SVG 元素组合**:
  - 主板 `<circle>` 大圆形（机芯底板）
  - 螺丝 4 颗：`<circle>` 嵌套 + `<line>` 一字槽
  - 齿轮组：4-5 个不同尺寸 `<circle>` + `<rect transform=rotate>` 齿牙（每齿轮 32-60 齿）
  - 发条盒：嵌套 `<circle>` 4 层（模拟发条螺旋）
  - 擒纵叉 `<polygon>` 不规则
  - 摆轮 `<circle>` + 4 根辐条 `<line>`
  - 游丝 `<polyline>` 60 段（阿基米德螺线）
  - 8 条标注引线 `<line>` + `<circle>` 端点 + `<text>` 中英文零件名
  - 底部 `<rect>` + `<text>` 传动逻辑说明
- **关键技术点**: 用 `<rect transform=rotate>` + 极坐标生成齿牙；polyline 模拟阿基米德螺线
- **推荐路径**: C

### 14. 赛博朋克城市夜景 · Cyberpunk Cityscape

- **示例 token**: `board_token_xxx`
- **节点数**: 1503（原文档）/ 1984（复刻）
- **类型分布**: composite_shape:1919（窗户密集）/ connector:48 / svg:9 / text_shape:16
- **设计模式**: 多层视差建筑剪影 + 高密度霓虹
- **SVG 元素组合**:
  - 深色 `<linearGradient>` 渐变背景
  - 月亮 `<radialGradient>` 光晕 + 内核 `<circle>`
  - 80 颗星星 `<circle>` 极小（r=0.5-1）
  - 远景建筑 `<path>` 多边形剪影（最暗 + 低 opacity）
  - 远景窗户 `<rect>` 3×4 小色块（暗灯）
  - 中景建筑 12 栋 `<rect>` 紫色高楼
  - 中景窗户：每栋楼 ~50-100 个 `<rect>` 多色（粉 / 紫 / 黄 / 青 / 金 5 色循环）
  - 中景顶部霓虹招牌 3 块 `<rect>` 边框 + `<text>` 招牌名
  - 近景建筑 N 栋 `<rect>` 最深色
  - 近景窗户：每栋楼 30-50 个 `<rect>`
  - 飞行器：4 架 `<polygon>` + 光迹 `<line>`
  - 全息广告投影：`<polygon>` 光锥 + `<text>` 投影文字
  - 雨滴 40 根 `<line>`
  - 地面反光 `<rect>` + 40 个 `<rect>` 色块
- **关键技术点**: 大量小 `<rect>` 重复模拟窗户（程序化生成）；3 层视差营造深度感
- **推荐路径**: C
- **性能边界**: 1984 节点是飞书画板的可承受上限附近，编辑器轻微卡顿

---

## 设计模式索引（按视觉模式查找）

| 模式 | 推荐示例 | 关键 SVG 元素 | 数学/算法 |
|------|---------|--------------|---------|
| 极坐标分布 | 飞轮、雷达 | `<circle>` + `<path>` 扇形 | `(cx+r·cosθ, cy+r·sinθ)` |
| 对称斜骨 | 鱼骨 | `<line>` 主骨 + 斜骨 | 三角函数等距挂载 |
| 层级递减 | 金字塔、漏斗 | `<polygon>` | 等差递减宽度 |
| 横向时间轴 | 路线图 | `<line>` + `<circle>` | 等距分布 |
| UI Mockup 嵌套 | Dashboard、Mobile UI | `<rect rx>` + 多组件 | 栅格布局 |
| 多层视差 | 插画、赛博朋克 | 多层 `<polygon>` / `<rect>` | 远近 z_index 堆叠 + opacity |
| 密集网格 | 周期表、地铁 | 大量 `<rect>` + `<text>` | 程序化循环生成 |
| 机械结构 | 机芯 | 嵌套 `<circle>` + `<rect transform=rotate>` | 极坐标 + rotate |
| 平面建筑 | 户型图 | 嵌套 `<rect>` + `<path>` 弧 | 比例尺标注 |
| 海报构图 | AI Builder Summit | `<linearGradient>` + 巨字 + `<polygon>` 斜切 | 视觉权重平衡 |
| 流条分布 | 桑基 | cubic-bezier `<path>` | 控制点 cx=(x1+x2)/2 |
| 信息层叠 | 商业图表 | `<rect rx>` 卡片 + `<text>` 阶层字号 | 8/12/16/24px 阶 |

---

## 节点密度排行（按规模）

| 排名 | 图 | 节点数 |
|------|-----|-------|
| 1 | 赛博朋克 | 1984 |
| 2 | 周期表 | 647 |
| 3 | 机芯 | 316 |
| 4 | 户型图（原） | 288 |
| 5 | Dashboard | 171 |
| 6 | 海报 | 124 |
| 7 | 平面图 | 119 |
| 8 | 地铁 | 117 |
| 9 | 插画 | 101 |
| 10 | 路线图 | 93 |
| 11 | 桑基 | 85 |
| 12 | 鱼骨 | 67 |
| 13 | Mobile UI | 67 |
| 14 | 飞轮 | 30 |
| 15 | 金字塔 | 28 |

---

## 一句话总结

| 用户需求 | 应该输出的图 |
|---------|------------|
| 画个商业模型 | 飞轮 / 金字塔 / 漏斗 / 桑基 |
| 画个根因分析 | 鱼骨 |
| 画产品计划 | 路线图 / 甘特 |
| 画 UI mockup | Dashboard / Mobile UI |
| 画建筑布局 | 户型图 / 地铁 |
| 画艺术创作 | 插画 / 海报 / 赛博朋克 |
| 画密集数据 | 周期表 |
| 画机械结构 | 机芯 |

所有 14 张都用同一条路径：**Python/AI 生成 SVG → `scripts/svg_to_board.py`**。
