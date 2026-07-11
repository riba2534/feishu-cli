# 飞书画板三大致命陷阱

这是从实战中沉淀的"血泪教训"，每个陷阱都让一张完整复杂的画板渲染失败过。把节点上传成功 ≠ 画板呈现正确，下面三个陷阱是导致"实际渲染和预期不符"的最常见根因。

如果你在做 SVG → 原生节点（路径 C）或 Mermaid 本地引擎（路径 B），**所有这三个陷阱都会踩到**——`scripts/svg_to_board.py` 已经把这三个修复内置成 5 步管道，能用一键脚本就别手写。

---

## 陷阱 1: z_index 错乱 ⭐⭐⭐

**严重程度：致命**。这个陷阱不会让命令报错，但画板视觉上一塌糊涂——大背景把前景节点全盖住、上千窗户消失、看起来"右半区一片空白"。

### 现象

- 渲染缩略图（`feishu-cli board image`）大片空白，看似节点没上传
- 实际查节点（`feishu-cli board nodes`）发现节点都在
- 飞书画板编辑器里手动选中能找到所有节点，但视觉堆叠错乱
- 大尺寸 dark 节点（背景）反而盖住了前景的所有小节点

### 根因

`whiteboard-cli -t openapi` 输出的节点 **不带 `z_index` 字段**。飞书 `create_nodes` API 在节点缺 z_index 时会自动分配——而**自动分配的顺序是无序的**（实测发现 1600×900 的背景大矩形被分到 z=1684 这种中高位，反而盖住了 z=0~1500 的所有元素）。

SVG 的"画家算法"语义是「先画在底、后画在上」，但飞书不知道你 SVG 文档的顺序。

### 验证

```bash
./feishu-cli board nodes <board_id> 2>/dev/null \
  | jq '[.data.nodes[] | {z:.z_index, t:.type, w:.width, h:.height, fill:.style.fill_color}] | sort_by(.z) | .[0:5]'
```

**预期**：z_index 最低的应该是大背景节点（width >= viewBox_w 的那个）。
**异常信号**：最低 z 是小窗户（width < 20），说明乱序。

### 修复

按 JSON 数组顺序显式赋 z_index（画家算法：数组前面的在底层）：

```python
import json
data = json.load(open("nodes.json"))
nodes = data["nodes"]
for i, node in enumerate(nodes):
    node["z_index"] = i        # 0..N-1，前小后大
json.dump(data, open("nodes.json", "w"))
```

### 一键修复

`scripts/svg_to_board.py` 的 Step 2 已内置此修复，无需手动操作。

---

## 陷阱 2: viewBox 溢出 ⭐⭐

**严重程度：高**。视觉上出现"半截楼"诡异图形——右边缘或下边缘一个细长条 / 残缺色块。用户最容易第一眼看出来。

### 现象

- 画板右边缘出现深色细长条
- 右下角有"半截楼"形状的诡异色块
- 边缘出现奇怪的纵向 / 横向条带

### 根因

`whiteboard-cli` 翻译 SVG 时，节点的 `x + width` 可能超过 SVG 的 viewBox 边界（设计 SVG 时元素紧贴右边或略超出，翻译后变成节点 `x=1540, width=82` → 右端 1622 > viewBox 1600）。

飞书画板按节点的真实包围盒计算 viewport，但渲染时按 viewBox 裁剪，导致超出部分被切成奇怪形状。

### 验证

```bash
./feishu-cli board nodes <board_id> 2>/dev/null \
  | jq '[.data.nodes[] | (.x + .width)] | max'
```

**预期**：≤ viewBox_w（SVG 的 viewBox 宽度）。
**异常信号**：超过 viewBox_w，每多 50 像素就是一个"半截楼"风险。

```bash
# 找具体的溢出节点
./feishu-cli board nodes <board_id> 2>/dev/null \
  | jq '[.data.nodes[] | select(.x + .width > 1600 or .y + .height > 900)] | length'
```

### 修复

上传前过滤 / 截断超出节点：

```python
def trim_overflow(nodes, vw, vh):
    kept = []
    for n in nodes:
        x = float(n.get("x", 0) or 0)
        y = float(n.get("y", 0) or 0)
        w = float(n.get("width", 0) or 0)
        h = float(n.get("height", 0) or 0)
        if x >= vw or y >= vh or (x + w) <= 0 or (y + h) <= 0:
            continue   # 完全在外 → 删
        # svg 节点截断会扭曲渲染（svg_code 与节点 width 绑定）→ 直接删
        if (x + w > vw or y + h > vh) and n.get("type") == "svg":
            continue
        # composite_shape 等几何节点 → 截断
        if x + w > vw:
            n["width"] = vw - x
        if y + h > vh:
            n["height"] = vh - y
        kept.append(n)
    return kept
```

### 一键修复

`scripts/svg_to_board.py` 的 Step 3 已内置，默认开启。如不想修剪可加 `--keep-overflow`。

---

## 陷阱 3: 节点翻倍 ⭐⭐

**严重程度：中**。视觉上颜色加深 / 细节叠加 / 性能变卡，但单看缩略图可能不明显。坑在脚本逻辑上，不在数据本身。

### 现象

- 第一次清空重传后节点数 ×2（甚至 ×3）
- 画板的同一区域颜色比预期更深更密
- `board nodes | jq '.data.nodes | length'` 比 nodes.json 的节点数翻倍
- 飞书编辑器里同一位置选中能选到多个重叠的相同节点

### 根因

经典踩坑场景：
1. 脚本调用 `feishu-cli board create-notes` 返回多行 JSON 输出
2. 脚本用 `out.strip().split("\n")[-1]` 取最后一行解析 → 失败（最后一行只是 `}`）
3. 脚本认为"上传失败"，自动重试或人工再跑一次
4. **但 API 调用其实成功了**——飞书后端已经创建了节点
5. 重试一次 → 节点翻倍

### 验证

```bash
# 拉真实节点数与预期对比
ACTUAL=$(./feishu-cli board nodes <board_id> 2>/dev/null | jq '.data.nodes | length')
EXPECTED=$(jq '.nodes | length' nodes.json)
echo "actual=$ACTUAL expected=$EXPECTED ratio=$(echo "scale=2; $ACTUAL / $EXPECTED" | bc)"
```

**预期 ratio ≈ 1.0**。**异常信号**：ratio ≥ 1.5 说明翻倍。

### 修复

#### A. 已翻倍：清空重传

```bash
./feishu-cli board delete <board_id> --all
python3 scripts/svg_to_board.py drawing.svg <board_id>
```

#### B. 防止再发生：用容错 JSON 解析

不要 `split("\n")[-1]`。改为从 stdout 找 `{ ... }` 主块：

```python
def parse_json_loose(stdout):
    s = stdout.strip()
    start = s.find("{")
    end = s.rfind("}")
    if start < 0 or end < 0:
        return None
    try:
        return json.loads(s[start:end+1])
    except Exception:
        return None
```

并且：**rc=0 时无论解析是否成功都按"上传成功"计数**——因为 rc=0 表示 feishu-cli 内部已确认 API 返回成功，HTTP 层面没问题。

### 一键修复

`scripts/svg_to_board.py` 的 Step 4 已内置容错解析：

```python
# 见 scripts/svg_to_board.py 中的 parse_create_notes_response()
# rc=0 但解析失败 → 按 len(chunk) 成功计，避免重试导致翻倍
```

---

## 通用诊断流程

任何画板呈现不符预期时，按以下 5 步排查：

### Step 1：拉节点总览

```bash
./feishu-cli board nodes <board_id> 2>/dev/null \
  | jq '.data.nodes | {total: length, types: ([.[].type] | group_by(.) | map({k:.[0],n:length}))}'
```

判断：节点总数对不对？类型分布合理吗？

### Step 2：看 z_index 分布

```bash
./feishu-cli board nodes <board_id> 2>/dev/null \
  | jq '[.data.nodes[].z_index] | {min:min, max:max, distinct:length}'
```

判断：最小 z 是 0 吗？distinct 数量 ≈ 总节点数吗？

```bash
./feishu-cli board nodes <board_id> 2>/dev/null \
  | jq '[.data.nodes[] | {z:.z_index, t:.type, w:.width}] | sort_by(.z) | .[0:5]'
```

判断：z 最低的几个节点是不是大背景？

### Step 3：看坐标范围

```bash
./feishu-cli board nodes <board_id> 2>/dev/null \
  | jq '[.data.nodes[]] | {x_min: ([.[].x] | min), x_max: ([.[].x] | max), x_w_max: ([.[] | (.x + .width)] | max), y_min: ([.[].y] | min), y_h_max: ([.[] | (.y + .height)] | max)}'
```

判断：x_w_max ≤ viewBox_w 吗？

### Step 4：看节点数翻倍

对比上传前 nodes.json 节点数和实际画板节点数。ratio 是否 ≈ 1.0？

### Step 5：拉缩略图人工目检

```bash
./feishu-cli board image <board_id> /tmp/check.png
```

注意：**缩略图也有渲染上限**。如果上面 1-4 都正常但缩略图缺东西，可能是飞书缩略图服务的限制——直接进飞书画板编辑器看真实渲染。

---

## 一句话总结

| 陷阱 | 一句话修复 |
|------|----------|
| z_index 错乱 | 上传前按数组 index 显式赋 `z_index = i` |
| viewBox 溢出 | 上传前过滤 `x+w > viewBox_w` 的节点 |
| 节点翻倍 | rc=0 一律按成功，错也别重试；已翻倍 → delete --all 后重传 |

或者更简单：**直接用 `scripts/svg_to_board.py`，三个修复都内置了**。
