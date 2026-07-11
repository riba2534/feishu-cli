#!/usr/bin/env python3
"""SVG → 飞书画板一键工作流（5 步管道）

把一张 SVG 翻译成 N 个独立的飞书画板节点（每个矢量元素都可单独点击编辑）。

5 步管道
========
Step 1: whiteboard-cli 翻译 SVG → 节点 JSON
Step 2: 修 z_index（按 JSON 数组顺序，画家算法 - 修陷阱 1）
Step 3: 修剪 viewBox 溢出节点（避免"半截楼" - 修陷阱 2）
Step 4: 分批 create-notes 上传（每批 300，间隔 0.3s）
Step 5: 验证 + 报告

依赖
====
- whiteboard-cli (npm i -g @larksuite/whiteboard-cli)
- feishu-cli（已 auth）

用法
====
    python3 svg_to_board.py drawing.svg <whiteboard_id>
    python3 svg_to_board.py drawing.svg <whiteboard_id> --viewbox 1600x900
    python3 svg_to_board.py drawing.svg <whiteboard_id> --batch 300 --interval 0.3
    python3 svg_to_board.py drawing.svg <whiteboard_id> --dry-run

退出码
======
    0 - 成功
    1 - whiteboard-cli 未安装或不可用
    2 - SVG 解析失败
    3 - feishu-cli 上传部分失败
"""

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
import xml.etree.ElementTree as ET


def fail(msg, code=1):
    print(f"\n❌ {msg}", file=sys.stderr)
    sys.exit(code)


def info(msg):
    print(f"  {msg}", flush=True)


def step(num, title):
    print(f"\n=== Step {num}: {title} ===", flush=True)


def parse_viewbox_from_svg(svg_path):
    """从 SVG 文件解析 viewBox 尺寸，返回 (w, h)。失败返回 None。"""
    try:
        tree = ET.parse(svg_path)
        root = tree.getroot()
        # 优先 viewBox
        vb = root.get("viewBox") or root.get("viewbox")
        if vb:
            parts = re.split(r"[\s,]+", vb.strip())
            if len(parts) == 4:
                w = float(parts[2])
                h = float(parts[3])
                if w > 0 and h > 0:
                    return w, h
        # 退化到 width/height
        w_attr = root.get("width", "").rstrip("px").rstrip("PX")
        h_attr = root.get("height", "").rstrip("px").rstrip("PX")
        if w_attr and h_attr:
            return float(w_attr), float(h_attr)
    except Exception:
        return None
    return None


def parse_viewbox_arg(arg):
    """解析 --viewbox 1600x900 形式参数。"""
    if not arg:
        return None
    m = re.match(r"^(\d+(?:\.\d+)?)[xX](\d+(?:\.\d+)?)$", arg.strip())
    if not m:
        fail(f"--viewbox 格式错误（应为 WxH，如 1600x900），收到：{arg}", 2)
    return float(m.group(1)), float(m.group(2))


def run(cmd, capture=True):
    """子进程调用，返回 (rc, stdout, stderr)。"""
    r = subprocess.run(cmd, capture_output=capture, text=True)
    return r.returncode, r.stdout, r.stderr


def step1_translate(svg_path, verbose):
    """调 whiteboard-cli 翻译 SVG → 节点 JSON。返回 nodes 数组。"""
    step(1, f"whiteboard-cli 翻译 SVG → 节点 JSON")
    if shutil.which("whiteboard-cli") is None:
        fail("whiteboard-cli 未安装。请运行：npm i -g @larksuite/whiteboard-cli", 1)

    tmp_out = tempfile.NamedTemporaryFile(suffix=".json", delete=False)
    tmp_out.close()
    try:
        cmd = ["whiteboard-cli", "-i", svg_path, "-f", "svg", "-t", "openapi", "-o", tmp_out.name]
        if verbose:
            cmd.append("-V")
        rc, out, err = run(cmd)
        if rc != 0:
            fail(f"whiteboard-cli 转换失败（rc={rc}）：{err.strip() or out.strip()}", 2)
        try:
            data = json.load(open(tmp_out.name))
        except Exception as e:
            fail(f"无法解析 whiteboard-cli 输出 JSON：{e}", 2)
        nodes = data.get("nodes")
        if not isinstance(nodes, list) or not nodes:
            # 退化兼容（数组本身或 data.nodes 包装）
            if isinstance(data, list):
                nodes = data
            elif isinstance(data.get("data"), dict):
                nodes = data["data"].get("nodes")
            if not isinstance(nodes, list) or not nodes:
                fail("whiteboard-cli 输出没有 nodes 字段或为空", 2)
        info(f"翻译成功：{len(nodes)} 个节点")
        # 类型分布
        type_count = {}
        for n in nodes:
            t = n.get("type", "?")
            type_count[t] = type_count.get(t, 0) + 1
        info(f"类型分布：{type_count}")
        return nodes
    finally:
        if os.path.exists(tmp_out.name):
            os.unlink(tmp_out.name)


def step2_fix_zindex(nodes):
    """陷阱 1 修复：按 JSON 数组顺序显式赋 z_index（画家算法）。"""
    step(2, "修 z_index（画家算法）")
    for i, node in enumerate(nodes):
        node["z_index"] = i
    info(f"已为 {len(nodes)} 个节点显式赋 z_index = 0..{len(nodes)-1}")
    return nodes


def step3_trim_overflow(nodes, vw, vh, keep_overflow):
    """陷阱 2 修复：修剪 x+width > vw 或 y+height > vh 的溢出节点。"""
    step(3, f"修剪 viewBox 溢出（{vw}x{vh}）")
    if keep_overflow:
        info("--keep-overflow 已设置，跳过修剪")
        return nodes

    kept = []
    removed = 0
    trimmed = 0
    for node in nodes:
        x = float(node.get("x", 0) or 0)
        y = float(node.get("y", 0) or 0)
        w = float(node.get("width", 0) or 0)
        h = float(node.get("height", 0) or 0)
        # 完全在 viewBox 外
        if x >= vw or y >= vh or (x + w) <= 0 or (y + h) <= 0:
            removed += 1
            continue
        # 左/上越界 → 截断
        if x < 0:
            new_w = max(1.0, w + x)
            node["x"] = 0
            node["width"] = new_w
            x, w = 0, new_w
            trimmed += 1
        if y < 0:
            new_h = max(1.0, h + y)
            node["y"] = 0
            node["height"] = new_h
            y, h = 0, new_h
            trimmed += 1
        # 右/下越界
        if x + w > vw:
            # svg 节点的 svg_code 内部坐标与节点 width 绑定，截断会扭曲渲染 → 直接删
            if node.get("type") == "svg":
                removed += 1
                continue
            new_w = vw - x
            if new_w < 1:
                removed += 1
                continue
            node["width"] = new_w
            trimmed += 1
        if y + h > vh:
            if node.get("type") == "svg":
                removed += 1
                continue
            new_h = vh - y
            if new_h < 1:
                removed += 1
                continue
            node["height"] = new_h
            trimmed += 1
        kept.append(node)
    info(f"保留 {len(kept)}，删除 {removed} 个完全溢出节点，截断 {trimmed} 个边缘节点")
    return kept


def parse_create_notes_response(stdout):
    """容错解析 board create-notes 的 JSON 输出（防陷阱 3 ↔ 翻倍）。"""
    s = stdout.strip()
    start = s.find("{")
    end = s.rfind("}")
    if start < 0 or end < 0:
        return None
    try:
        return json.loads(s[start:end+1])
    except Exception:
        return None


def step4_upload(nodes, board_id, feishu_cli, batch, interval):
    """分批 create-notes 上传。"""
    step(4, f"分批上传（batch={batch} interval={interval}s）")
    if batch <= 0:
        fail("--batch 必须大于 0", 2)
    if interval < 0:
        fail("--interval 不能小于 0", 2)
    total = len(nodes)
    if total == 0:
        info("无节点可上传")
        return 0, []
    n_ok = 0
    n_fail = 0
    failed_batches = []
    for i in range(0, total, batch):
        chunk = nodes[i:i+batch]
        with tempfile.NamedTemporaryFile(suffix=".json", delete=False, mode="w") as tmp:
            json.dump(chunk, tmp)
            tmp_path = tmp.name
        try:
            rc, out, err = run([feishu_cli, "board", "create-notes", board_id, tmp_path, "-o", "json"])
            if rc != 0:
                info(f"✗ 批 {i}-{i+len(chunk)} 失败: rc={rc} {err.strip()[:160]}")
                n_fail += len(chunk)
                failed_batches.append((i, i+len(chunk)))
                continue
            parsed = parse_create_notes_response(out)
            if parsed and parsed.get("count") is not None:
                cnt = parsed["count"]
                n_ok += cnt
                if cnt != len(chunk):
                    info(f"⚠ 批 {i}-{i+len(chunk)} API 返回 count={cnt} != 提交 {len(chunk)}")
            else:
                # rc=0 但解析失败：仍按成功计（这是陷阱 3 的根因，避免重传翻倍）
                n_ok += len(chunk)
                info(f"⚠ 批 {i}-{i+len(chunk)} 输出解析失败但 rc=0，按成功计")
            info(f"✓ 批 {i}-{i+len(chunk)} 上传 {len(chunk)}")
        finally:
            if os.path.exists(tmp_path):
                os.unlink(tmp_path)
        if i + batch < total and interval > 0:
            time.sleep(interval)
    info(f"上传完成：{n_ok}/{total}（失败 {n_fail}）")
    return n_ok, failed_batches


def step5_verify(board_id, feishu_cli, expected_count):
    """读取画板验证节点数。"""
    step(5, "验证")
    rc, out, err = run([feishu_cli, "board", "nodes", board_id])
    if rc != 0:
        info(f"⚠ 验证失败（不影响已上传节点）：{err.strip()[:160]}")
        return
    try:
        s = out.strip()
        start = s.find("{")
        end = s.rfind("}")
        if start >= 0 and end >= 0:
            data = json.loads(s[start:end+1])
            actual_nodes = data.get("data", {}).get("nodes", []) or []
            if isinstance(actual_nodes, dict):
                actual_nodes = list(actual_nodes.values())
            actual = len(actual_nodes)
            type_count = {}
            for n in actual_nodes:
                t = n.get("type", "?")
                type_count[t] = type_count.get(t, 0) + 1
            info(f"画板节点数：{actual}（期望 ≈ {expected_count}）")
            info(f"类型分布：{type_count}")
            if actual >= expected_count * 1.5:
                info(f"⚠ 实际 >> 期望，可能发生翻倍（陷阱 3）。建议 board delete --all 后重传")
    except Exception as e:
        info(f"⚠ 验证响应解析失败：{e}")


def main():
    parser = argparse.ArgumentParser(
        description="SVG → 飞书画板一键工作流（每个矢量元素 = 1 个独立可编辑节点）",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("svg_path", help="SVG 文件路径")
    parser.add_argument("board_id", help="目标画板 ID")
    parser.add_argument("--feishu-cli", default="feishu-cli", help="feishu-cli 命令路径（默认在 PATH 中查找）")
    parser.add_argument("--batch", type=int, default=300, help="每批节点数（默认 300）")
    parser.add_argument("--interval", type=float, default=0.3, help="批间隔秒（默认 0.3）")
    parser.add_argument("--viewbox", default=None, help="viewBox 尺寸 WxH（默认从 SVG 解析）")
    parser.add_argument("--keep-overflow", action="store_true", help="不裁剪 viewBox 溢出节点")
    parser.add_argument("--dry-run", action="store_true", help="只跑 Step 1-3，不上传")
    parser.add_argument("-v", "--verbose", action="store_true", help="详细日志")
    args = parser.parse_args()

    if not os.path.exists(args.svg_path):
        fail(f"SVG 文件不存在：{args.svg_path}", 2)

    # 解析 viewBox 尺寸
    if args.viewbox:
        vb = parse_viewbox_arg(args.viewbox)
    else:
        vb = parse_viewbox_from_svg(args.svg_path)
    if not vb:
        fail("无法自动解析 SVG viewBox，请用 --viewbox WxH 显式指定", 2)
    vw, vh = vb
    info(f"viewBox = {vw}x{vh}")

    # 校验 feishu-cli
    if not args.dry_run and shutil.which(args.feishu_cli) is None:
        fail(f"feishu-cli 不可用（{args.feishu_cli}）。请确认安装并在 PATH 中", 1)

    t0 = time.time()
    # Step 1
    nodes = step1_translate(args.svg_path, args.verbose)
    # Step 2
    nodes = step2_fix_zindex(nodes)
    # Step 3
    nodes = step3_trim_overflow(nodes, vw, vh, args.keep_overflow)
    if not nodes:
        fail("修剪后节点数为 0，无可上传内容", 2)

    if args.dry_run:
        print(f"\n[dry-run] 跳过 Step 4-5。将上传 {len(nodes)} 个节点到画板 {args.board_id}")
        return

    # Step 4
    n_ok, failed_batches = step4_upload(nodes, args.board_id, args.feishu_cli, args.batch, args.interval)
    # Step 5
    step5_verify(args.board_id, args.feishu_cli, n_ok)

    elapsed = time.time() - t0
    print(f"\n========== 完成（{elapsed:.1f}s）==========")
    print(f"画板：https://feishu.cn/wiki/wikcn ⟂ 或 docx 嵌入位置")
    print(f"节点：{n_ok} 个独立可编辑节点已落到 {args.board_id}")

    if failed_batches:
        print(f"\n⚠ {len(failed_batches)} 批失败：{failed_batches}")
        sys.exit(3)


if __name__ == "__main__":
    main()
