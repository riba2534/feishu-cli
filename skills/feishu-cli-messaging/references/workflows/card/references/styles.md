# 卡片视觉风格路由

场景模板决定“放什么信息”，视觉预设决定“如何呈现”。先选
`templates/*.json` 的业务骨架，再独立选择视觉预设；不要把“告警”“审批”误当成视觉风格。

本 Skill 内置 19 个可选择的预设：

- 1 个默认语义风格；
- 6 个 Dashboard 子风格；
- 1 套 Modern Minimal 设计系统；
- 11 个 Custom Visual 命名主题。

完整图片素材位于 `../assets/`：5 张头图、153 个 doodle 图标、48 个 phosphor 图标。
使用前先运行：

```bash
python3 ../scripts/style_assets.py verify
python3 ../scripts/style_assets.py styles
```

## 1. 选择顺序

1. 先根据任务选择通知、成功、告警、审批、数据、长文摘要或流式输出场景骨架。
2. 用户点名风格或风格词时，按下表选择 `style_id`；未指定则使用 `default-semantic`。
3. 用 `style_assets.py show <style_id>` 读取预设摘要，再加载对应的详细参考。
4. 用真实内容删除不需要的模块。预设是设计约束，不是必须填满的表单。

## 2. 路由表

| 用户表达 | `style_id` / 家族 | 必读参考 |
|---|---|---|
| 未指定、普通通知、状态同步 | `default-semantic` | 本文件“默认语义” |
| dashboard、看板、经营汇报、监控态势 | `dashboard-*` | `styles/dashboard.md` |
| 现代极简、Stripe-like、Linear-like、SaaS/API 平台 | `modern-minimal` | `styles/modern-minimal/index.md`，再读 `basic.md` 和 `components.md` |
| 点名 Lime Slab 等主题、手绘、暗黑、杂志、海报、品牌页 | 对应 Custom Visual `style_id` | `styles/custom-visual/index.md`、`design.md` 和 palette manifest |

仅出现 KPI、指标、图表、日报或周报时，不自动套 Dashboard；主题需要用户明确点选。仅出现
“简洁”也不等于 Modern Minimal，除非用户明确要求现代、极简、克制或对应产品视觉。

## 3. 默认语义风格

适合通知、告警、审批和状态同步，是未指定风格时的默认值。

- header 色表达状态：信息 `blue`、成功 `green`、注意 `orange`、故障 `red`、归档 `grey`。
- 第一段直接写结论；2–4 个关键字段使用 `div.fields`。
- 至多一个 primary 主行动；不需要行动时不放按钮。
- 次要详情用折叠面板，来源和时间用灰色 notation 文本。

```text
header → 结论 → 关键字段 → 必要详情 → 真实行动 → 来源
```

## 4. Dashboard 六个子风格

| `style_id` | 风格 | 首屏重点 |
|---|---|---|
| `dashboard-monitoring` | 监控态势型 | 状态、时间窗口、核心异常 |
| `dashboard-business` | 经营汇报型 | 一句业务结论、KPI 达成、主要变化 |
| `dashboard-trend` | 趋势分析型 | 趋势方向、拐点、原因 |
| `dashboard-ranking` | 排行洞察型 | 榜首/末尾、最大变化、异常项 |
| `dashboard-health` | 健康评分型 | 评分、等级、主要扣分项 |
| `dashboard-minimal` | 极简概览型 | 一句结论、3–4 个指标、一个风险或下一步 |

详细规则见 `styles/dashboard.md`。

## 5. Modern Minimal

适合克制、留白、编辑感、SaaS 官网、API 平台和企业工具。它不是“把文字变灰”，而是用
固定 token 和组件配方限制视觉选择。

主要模块：

```text
Cover Header → Split Hero → optional Key Point Columns
→ Notice/Surface/Metric/Media/Details → Footer
```

- 5 张语义头图：`info`、`success`、`warning`、`danger`、`neutral`；
- 48 个 phosphor 线性图标；
- 完整颜色、间距、字体、表面和无障碍 token；
- Cover Header、Status Pill、Split Hero、Key Point Columns、Notice Band、Surface Card、
  Metric Section、Media List、Collapsible Digest、Review Form、Footer Note、Clickable Surface。

先读 `styles/modern-minimal/index.md`，然后完整读取其指定的 `basic.md` 与 `components.md`。

## 6. Custom Visual 十一个主题

| `style_id` | 视觉承诺 | 推荐场景 |
|---|---|---|
| `lime-slab` | electric lime + warm charcoal | 产品说明、功能拆解、对比网格 |
| `editorial-forest` | cream + forest + dusty rose | 策略说明、年度报告、编辑摘要 |
| `monochrome` | cream + ink，无强调色 | 高管摘要、政策说明、严肃备忘 |
| `macchiato` | almond + espresso + taupe | 温和总结、知识说明、评审文档 |
| `neo-grid-bold` | 严格网格 + neon lemon | 发布 brief、指标网格、结构化报告 |
| `court-press` | grass green + dusty pink | 团队更新、步骤说明、活动复盘 |
| `soft-editorial` | warm magazine + soft pastels | 研究摘要、温和概览、编辑化日报 |
| `handdrawn-vibes` | cream paper + blue/yellow doodle | 轻量状态、创作者更新、轻产品介绍 |
| `handdrawn-dark` | near-black + neon/terracotta | 夜间状态、暗黑手绘、科技表达 |
| `wellness-planner` | cream + olive + lifestyle accents | 健康习惯、餐食计划、每日安排 |
| `claude-chrome` | off-white + deep cream + mint | 产品能力、工具说明、官网风信息卡 |

结构原型分为 Editorial Showcase、Handdrawn Status、Wellness Planner 和 Product Showcase。
完整规则见 `styles/custom-visual/index.md` 与 `styles/custom-visual/design.md`；唯一颜色数据源是
`../assets/themes/custom-visual-palettes.json`。

## 7. 素材选择与直接复制

列出或检索：

```bash
python3 ../scripts/style_assets.py styles
python3 ../scripts/style_assets.py show handdrawn-dark
python3 ../scripts/style_assets.py assets --family header
python3 ../scripts/style_assets.py assets --family doodle --query trend
python3 ../scripts/style_assets.py assets --family phosphor --query warning
```

复制一个素材：

```bash
python3 ../scripts/style_assets.py \
  copy doodle:trend-up.safe16 \
  --output /tmp/trend-up.png
```

生成 Custom Visual 可直接注入的明暗模式 token：

```bash
python3 ../scripts/style_assets.py \
  tokens handdrawn-dark \
  --output /tmp/handdrawn-dark-tokens.json
```

生成完整起稿：

```bash
python3 ../scripts/style_assets.py \
  starter handdrawn-dark \
  --title "夜间构建状态" \
  --summary "主链路已恢复" \
  --detail "剩余两项非阻断任务继续观察" \
  --meta "构建窗口 · UTC+8" \
  --output /tmp/night-build-card.json
```

起稿里的素材路径指向本 Skill 内文件。可以直接保留，也可以先用 `copy` 复制到业务目录。
发送前必须同时在 lint 和 `msg send` 使用 `--upload-images`：

```bash
python3 ../scripts/lint_card.py \
  --strict --upload-images /tmp/night-build-card.json

feishu-cli msg send \
  --receive-id-type chat_id \
  --receive-id oc_xxx \
  --msg-type interactive \
  --content-file /tmp/night-build-card.json \
  --upload-images
```

CLI 会把 `img.img_key`、`img_combination.img_list[].img_key` 和 Markdown 图片中的本地路径
上传并替换为当前 App 可用的图片 key。素材缺失、内容不是受支持图片或上传失败时会立即中止，
不会把本地绝对路径继续送入发消息 API。

## 8. 共同约束

- 预制图片提供视觉能力，不提供业务事实；不得从示例反推数据、人员、链接或行动。
- 只在真实数据关系存在时使用 chart；图表不比两行文字清楚时删除。
- 图片必须有语义化 `alt`；颜色状态必须同时有文字说明。
- `collapsible_panel.header` 默认使用 `white`、`default` 或 `*-50/*-100` 浅色背景；
  不使用饱和语义色横条。需要呼应卡片主色时，给标题 markdown 的 `<font>`、折叠图标或
  细边框上色。
- 移动端堆叠后的阅读顺序必须成立，关键结论不能只放右侧分栏。
- 没有真实 URL、回调或表单处理器就删除行动区。
- 发送候选必须通过 `lint_card.py --strict`；包含本地素材时追加 `--upload-images`。
