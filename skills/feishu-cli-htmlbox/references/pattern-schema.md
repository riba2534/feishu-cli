# Animated Flowchart — Pattern Schema

`scripts/animate_diagram.py` 接受如下形状的 JSON 对象：

```json
{
  "title": "Supervisor / Manager",
  "sub": "Agents-as-tools · centralized orchestration",
  "nodes": [
    {
      "id": "manager",
      "x": 230,
      "y": 230,
      "w": 180,
      "h": 54,
      "label": "Manager",
      "sub": "supervisor",
      "kind": "accent"
    }
  ],
  "edges": {
    "m-c": {
      "from": "manager",
      "to": "coder",
      "label": "call",
      "curve": 0,
      "dashed": false
    }
  },
  "timeline": [
    {
      "caption": "Manager calls <b>Coder</b> to write code.",
      "fire": ["m-c"],
      "activate": ["manager", "coder"],
      "dim": [],
      "duration": 1500
    }
  ]
}
```

## 必填字段

- `title`：图标题。
- `nodes`：节点对象数组。
- `edges`：以 edge ID 为 key 的对象。
- `timeline`：有序的动画步骤。

## 节点字段

- `id` 必填，唯一。
- `x`、`y` 必填，节点左上角坐标（参考坐标系 `900 x 540`；最终 viewBox 会按所有节点的包围盒 + padding 自动收紧，无需把节点手动铺满整个画布）。
- `label` 必填。
- `w` 可选，默认 `140`。
- `h` 可选，默认 `54`。
- `sub` 可选，小号大写副标题。
- `kind` 可选：`plain`、`accent`、`dark`、`user`、`store`、`bus`。

## 连线字段

- `from` 必填，节点 ID。
- `to` 必填，节点 ID。
- `label` 可选。
- `curve` 可选数字。正负值让连线朝相反方向弯曲。
- `dashed` 可选布尔值。

## 时间线字段

- `caption` 必填。允许内联 `<b>` 和 `<code>`。
- `fire` 可选，edge ID 数组。前缀 `!` 表示 token 反向流动。
- `activate` 可选，要脉冲 / 高亮的节点 ID 数组。
- `dim` 可选，要淡出的节点 ID 数组。
- `duration` 可选，毫秒，默认 `1500`。

## 设计默认值

- 固定浅色主题。
- 默认自动播放。
- 极简控件：上一步、播放 / 暂停、下一步、进度点。
- 默认无键盘快捷键、倍速控件、重播按钮、外部资源或运行时依赖。
