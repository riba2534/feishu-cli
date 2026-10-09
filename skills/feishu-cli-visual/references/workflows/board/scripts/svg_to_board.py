#!/usr/bin/env python3
"""SVG → 飞书画板一键工作流（5 步管道）

把一张 SVG 翻译成 N 个独立的飞书画板节点（每个矢量元素都可单独点击编辑）。

5 步管道
========
Step 1: whiteboard-cli 翻译 SVG → 节点 JSON
Step 2: 修 z_index（按 JSON 数组顺序，画家算法 - 修陷阱 1）
Step 3: 修剪 viewBox 溢出节点（避免"半截楼" - 修陷阱 2）
Step 4: 分批 create-notes 上传（每批 300，间隔 0.3s；每批带确定性 client_token，重跑幂等）
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
    3 - 上传或回读验证未完成（先回读核对；补传时原样重跑本命令，已落地的批次不会重复创建）
"""

import argparse
import hashlib
import json
import math
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
    """返回完整的 (min_x, min_y, width, height)，不能丢掉非零或负原点。"""
    try:
        tree = ET.parse(svg_path)
        root = tree.getroot()
        # 优先 viewBox
        vb = root.get("viewBox") or root.get("viewbox")
        if vb:
            parts = re.split(r"[\s,]+", vb.strip())
            if len(parts) == 4:
                values = tuple(float(part) for part in parts)
                if all(math.isfinite(v) for v in values) and values[2] > 0 and values[3] > 0:
                    return values
            return None
        # 退化到 width/height
        w_attr = root.get("width", "").rstrip("px").rstrip("PX")
        h_attr = root.get("height", "").rstrip("px").rstrip("PX")
        if w_attr and h_attr:
            w, h = float(w_attr), float(h_attr)
            if math.isfinite(w) and math.isfinite(h) and w > 0 and h > 0:
                return 0.0, 0.0, w, h
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
    w, h = float(m.group(1)), float(m.group(2))
    if not math.isfinite(w) or not math.isfinite(h) or w <= 0 or h <= 0:
        fail("--viewbox 宽高必须是有限正数", 2)
    return 0.0, 0.0, w, h


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
            with open(tmp_out.name, encoding="utf-8") as output:
                data = json.load(output)
        except Exception as e:
            fail(f"无法解析 whiteboard-cli 输出 JSON：{e}", 2)
        nodes = data.get("nodes") if isinstance(data, dict) else None
        if not isinstance(nodes, list) or not nodes:
            # 退化兼容（数组本身或 data.nodes 包装）
            if isinstance(data, list):
                nodes = data
            elif isinstance(data, dict) and isinstance(data.get("data"), dict):
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


def step3_trim_overflow(nodes, vw, vh, keep_overflow, min_x=0.0, min_y=0.0):
    """按原始 viewBox 边界裁剪，保留复合节点内部的相对坐标。"""
    step(3, f"修剪 viewBox 溢出（原点 {min_x},{min_y}，{vw}x{vh}）")
    if keep_overflow:
        info("--keep-overflow 已设置，跳过修剪")
        return nodes

    kept = []
    removed = 0
    trimmed = 0
    max_x, max_y = min_x + vw, min_y + vh
    for node in nodes:
        clipped = False
        x = float(node.get("x", 0) or 0)
        y = float(node.get("y", 0) or 0)
        w = float(node.get("width", 0) or 0)
        h = float(node.get("height", 0) or 0)
        # 完全在 viewBox 外
        if x >= max_x or y >= max_y or (x + w) <= min_x or (y + h) <= min_y:
            removed += 1
            continue
        # 左/上越界 → 截断
        if x < min_x:
            new_w = max(1.0, w + x - min_x)
            node["x"] = min_x
            node["width"] = new_w
            x, w = min_x, new_w
            clipped = True
        if y < min_y:
            new_h = max(1.0, h + y - min_y)
            node["y"] = min_y
            node["height"] = new_h
            y, h = min_y, new_h
            clipped = True
        # 右/下越界
        if x + w > max_x:
            # svg 节点的 svg_code 内部坐标与节点 width 绑定，截断会扭曲渲染 → 直接删
            if node.get("type") == "svg":
                removed += 1
                continue
            new_w = max_x - x
            if new_w < 1:
                removed += 1
                continue
            node["width"] = new_w
            clipped = True
        if y + h > max_y:
            if node.get("type") == "svg":
                removed += 1
                continue
            new_h = max_y - y
            if new_h < 1:
                removed += 1
                continue
            node["height"] = new_h
            clipped = True
        if clipped:
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


def batch_client_token(board_id, chunk):
    """按画板 ID + 本批节点内容生成确定性 client_token。

    同一 SVG 重跑时每批 token 不变：服务端对已落地的批次直接返回首次创建的节点 ID，
    只有上次没落地的批次会真正创建，避免整批重传导致节点翻倍（陷阱 3）。
    """
    payload = board_id + "\n" + json.dumps(chunk, sort_keys=True, ensure_ascii=False)
    return "svg2board-" + hashlib.sha256(payload.encode("utf-8")).hexdigest()[:32]


def step4_upload(nodes, board_id, feishu_cli, batch, interval):
    """分批 create-notes 上传（每批带确定性 client_token，重跑幂等）。"""
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
            rc, out, err = run([feishu_cli, "board", "create-notes", board_id, tmp_path, "-o", "json",
                                "--client-token", batch_client_token(board_id, chunk)])
            if rc != 0:
                info(f"✗ 批 {i}-{i+len(chunk)} 失败: rc={rc} {err.strip()[:160]}")
                n_fail += len(chunk)
                failed_batches.append((i, i+len(chunk)))
                continue
            parsed = parse_create_notes_response(out)
            cnt = parsed.get("count") if isinstance(parsed, dict) else None
            # exit 0 只说明命令没有报错；数量异常不能冒充整批创建成功。
            # 结果不确定时也不自动重传，避免已落地节点被重复创建。
            if type(cnt) is not int or not 0 <= cnt <= len(chunk):
                n_fail += len(chunk)
                failed_batches.append((i, i+len(chunk)))
                info(f"✗ 批 {i}-{i+len(chunk)} 结果不确定：输出无法解析或 count 无效；"
                     "请回读画板核对，未自动重传（补传时原样重跑本命令）")
            elif cnt != len(chunk):
                n_ok += cnt
                n_fail += len(chunk) - cnt
                failed_batches.append((i, i+len(chunk)))
                info(f"✗ 批 {i}-{i+len(chunk)} 创建不完整：确认 {cnt}/{len(chunk)} 个；"
                     "请回读画板核对，未自动重传（补传时原样重跑本命令）")
            else:
                n_ok += cnt
                info(f"✓ 批 {i}-{i+len(chunk)} 上传 {len(chunk)}")
        finally:
            if os.path.exists(tmp_path):
                os.unlink(tmp_path)
        if i + batch < total and interval > 0:
            time.sleep(interval)
    info(f"已确认创建：{n_ok}/{total}（失败或未确认 {n_fail}）")
    return n_ok, failed_batches


def step5_verify(board_id, feishu_cli, expected_count):
    """只检查节点总数下限；总数含已有节点，不能推断本批内容或重复。"""
    step(5, "回读验证（节点总数下限检查）")
    rc, out, err = run([feishu_cli, "board", "nodes", board_id])
    if rc != 0:
        info(f"✗ 回读验证未完成：{err.strip()[:160]}")
        return False
    data = parse_create_notes_response(out)
    payload = data.get("data") if isinstance(data, dict) else None
    actual_nodes = payload.get("nodes") if isinstance(payload, dict) else None
    if isinstance(actual_nodes, dict):
        actual_nodes = list(actual_nodes.values())
    if not isinstance(actual_nodes, list) or any(not isinstance(n, dict) for n in actual_nodes):
        info("✗ 回读验证未完成：响应缺少有效的 data.nodes 列表")
        return False
    actual = len(actual_nodes)
    type_count = {}
    for node in actual_nodes:
        node_type = node.get("type", "?")
        type_count[node_type] = type_count.get(node_type, 0) + 1
    info(f"画板节点总数：{actual}（含已有节点；已确认新增 {expected_count}）")
    info(f"类型分布：{type_count}")
    if actual < expected_count:
        info("✗ 回读验证未完成：节点总数少于已确认新增数，请核对画板")
        return False
    info("节点总数下限检查通过；不能据此确认每个新增节点的内容或是否重复")
    return True


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
    min_x, min_y, vw, vh = vb
    info(f"viewBox = {min_x} {min_y} {vw} {vh}")

    # 校验 feishu-cli
    if not args.dry_run and shutil.which(args.feishu_cli) is None:
        fail(f"feishu-cli 不可用（{args.feishu_cli}）。请确认安装并在 PATH 中", 1)

    t0 = time.time()
    # Step 1
    nodes = step1_translate(args.svg_path, args.verbose)
    # Step 2
    nodes = step2_fix_zindex(nodes)
    # Step 3
    nodes = step3_trim_overflow(nodes, vw, vh, args.keep_overflow, min_x, min_y)
    if not nodes:
        fail("修剪后节点数为 0，无可上传内容", 2)

    if args.dry_run:
        print(f"\n[dry-run] 跳过 Step 4-5。将上传 {len(nodes)} 个节点到画板 {args.board_id}")
        return

    # Step 4
    n_ok, failed_batches = step4_upload(nodes, args.board_id, args.feishu_cli, args.batch, args.interval)
    # Step 5
    verified = step5_verify(args.board_id, args.feishu_cli, n_ok)

    elapsed = time.time() - t0
    status = "未完成" if failed_batches or not verified else "完成"
    print(f"\n========== {status}（{elapsed:.1f}s）==========")
    print(f"节点：已确认 {n_ok} 个独立可编辑节点落到 {args.board_id}")

    if failed_batches:
        print(f"\n⚠ {len(failed_batches)} 批失败、不完整或结果不确定：{failed_batches}；"
              "请先回读核对，不要直接整批重传。补传时用同一 SVG、同一画板原样重跑本命令："
              "每批 client_token 不变，已落地的批次直接返回原节点，不会翻倍")
    if not verified:
        print("\n⚠ 回读验证未完成；请先核对已落地节点，不要直接整批重传（原样重跑本命令是幂等的）")
    if failed_batches or not verified:
        sys.exit(3)


if __name__ == "__main__":
    main()
