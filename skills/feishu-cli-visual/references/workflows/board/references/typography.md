# 排版规则

## 字号层级表

| 层级 | font_size | font_weight | horizontal_align | 用途 |
|------|-----------|-------------|-----------------|------|
| H1 标题 | 24-28 | `bold` | `center` | 图表标题（每图一个） |
| H2 分区 | 18-20 | `bold` | `center` 或 `left` | 分区标签、层标签 |
| H3 卡片标题 | 15-16 | `bold` | `center` | 分组标题、卡片标题 |
| Body 正文 | 14 | `regular` | `center` | 节点文字、正文内容 |
| Caption 注解 | 13 | `regular` | `left` | 辅助说明、注解 |

规则：
- 同张图不超过 3 个字号层级
- 同级节点 font_size 必须完全相同
- 相邻层级字号差 >= 4px

---

## 对齐规则

| 内容类型 | horizontal_align | vertical_align |
|---------|-----------------|---------------|
| 短文本（<= 15 字） | `center` | `mid` |
| 长文本（> 15 字） | `left` | `mid` |
| 图表标题 | `center` | `mid` |
| 分区标签 | `left` 或 `center` | `mid` |
| 多行描述 | `left` | `top` |
| 背景分区标签 | `left` | `top` |

---

## 图表标题

用 composite_shape 模拟纯文本节点（无边框无填充），宽度设为图表整体宽度：

```json
{
  "type": "composite_shape",
  "x": 150, "y": 20, "width": 500, "height": 40,
  "z_index": 10,
  "composite_shape": {"type": "round_rect"},
  "text": {"text": "系统架构图", "font_size": 24, "font_weight": "bold",
           "horizontal_align": "center", "vertical_align": "mid"},
  "style": {"fill_opacity": 0, "border_style": "none"}
}
```

---

## 节点文字规范

**标题 + 简短说明**：用换行符分隔

```json
"text": {"text": "用户服务\n注册登录和权限管理", "font_size": 14}
```

- 标题：4-8 字，概括功能
- 说明：8-12 字，补充细节
- 不写长段落

**纯标题**：无需说明时只写标题

```json
"text": {"text": "API 网关", "font_size": 14, "font_weight": "bold"}
```

---

## 尺寸适配

文字越多，节点尺寸越大：

| 文字量 | 推荐 width | 推荐 height |
|--------|-----------|------------|
| 2-4 字 | 120 | 40 |
| 5-8 字 | 160 | 40 |
| 标题 + 1 行说明 | 180 | 55 |
| 标题 + 2 行说明 | 200 | 70 |

中文约 14px/字，英文约 8px/字。估算公式：

```
min_width = max(120, 字数 * 14 + 24)   # 24 为左右内边距
min_height = max(40, 行数 * 20 + 16)    # 20 为行高，16 为上下内边距
```
