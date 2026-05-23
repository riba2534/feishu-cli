#!/usr/bin/env python3
"""
端到端拉取飞书群聊（含话题群）的消息记录。

实战中 `feishu-cli msg history` + `msg thread-messages` 的输出存在多处不一致
（PascalCase vs snake_case、秒/毫秒、撤回消息为字符串、post 双结构、bot app_id
和 sender_names 中的 ou_xxx 不互通），这些怪癖单条命令解决不了，因此沉淀成
这个脚本。详细字段差异见 ../references/output-quirks.md。

用法
----
    # 拉最近 24 小时
    python fetch_chat_history.py oc_xxxxxxxxxxxxxxxxxxxxxxxxxxx --since 24h

    # 拉指定时间窗（接受 Nh/Nd 或 ISO8601 起止时间）
    python fetch_chat_history.py oc_xxx --since 2d
    python fetch_chat_history.py oc_xxx --start 2026-05-20T00:00:00 --end 2026-05-22T00:00:00

    # 自定义输出目录、关闭线程展开、显式 user token、自定义 cli 路径
    python fetch_chat_history.py oc_xxx --since 24h \
        --output-dir /tmp/my_chat \
        --no-thread \
        --user-access-token u-xxx \
        --cli /path/to/feishu-cli

输出
----
    <output-dir>/
        history.json    # 主消息原始 JSON
        threads.json    # 每个 thread_id → 回复列表
        names.json      # open_id / app_id → 名字
        timeline.txt    # 渲染后的可读时间线（主消息 + 缩进的线程回复）

为何用脚本而不是 CLI
--------------------
1. msg history 和 msg thread-messages 的 JSON key 风格不一致（snake_case vs PascalCase），
   单条 jq 处理不了；
2. msg history 时间用秒，msg thread-messages 时间用毫秒，循环翻页时容易写错；
3. 跨企业用户 user info 会返 41050（no user authority），需要降级到 mentions 字段，
   这一层降级不放代码里很容易漏；
4. interactive 卡片 v2 schema 的 body.elements 需要递归（含 column_set / form /
   collapsible_panel / action），手写 jq 太脆。
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


# ---------------------------------------------------------------------------
# CLI 调用层
# ---------------------------------------------------------------------------

def find_cli(explicit: str | None) -> str:
    """定位 feishu-cli 二进制。优先级：用户传入 → 仓库根（开发场景下最新编译产物）→ PATH。

    把仓库根放在 PATH 之前是关键：本 skill 在飞书 CLI 仓库内开发时，仓库根的二进制
    一定是刚 go build 的新版，而 PATH 上可能是更老的 go install 安装版（实战中遇到过
    旧版不会自动加载 ~/.feishu-cli/token.json，导致脚本调用回落 App Token 失败）。
    脚本被装到 ~/.claude/skills/ 等仓库外位置时，仓库根的 candidate 不存在，自动 fallback
    到 PATH，所以这个顺序对两类用户都安全。"""
    if explicit:
        return explicit
    # 脚本相对位置：<repo>/skills/feishu-cli-chat/scripts/fetch_chat_history.py
    repo_root = Path(__file__).resolve().parents[3]
    candidate = repo_root / "feishu-cli"
    if candidate.exists() and os.access(candidate, os.X_OK):
        return str(candidate)
    in_path = shutil.which("feishu-cli")
    if in_path:
        return in_path
    sys.exit("找不到 feishu-cli，请用 --cli 指定路径，或先 go build -o feishu-cli .")


def run_cli_json(cli: str, args: list[str], user_token: str | None = None) -> dict | list | None:
    """执行 CLI 子命令并解析为 JSON。返回 None 表示命令失败或非 JSON 输出。"""
    cmd = [cli, *args]
    if user_token:
        cmd += ["--user-access-token", user_token]
    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode != 0:
        # 调用方决定是否打印；这里仅在 verbose 时回显
        if os.environ.get("FETCH_CHAT_VERBOSE"):
            print(f"[ERR] {' '.join(args)}\n{res.stderr.strip()}", file=sys.stderr)
        return None
    out = res.stdout
    try:
        return json.loads(out)
    except json.JSONDecodeError:
        # CLI 偶尔在 JSON 前打印日志行，找到第一个 { / [
        idx_obj = out.find("{")
        idx_arr = out.find("[")
        candidates = [i for i in (idx_obj, idx_arr) if i >= 0]
        if candidates:
            try:
                return json.loads(out[min(candidates):])
            except Exception:
                return None
        return None


# ---------------------------------------------------------------------------
# 时间窗口解析
# ---------------------------------------------------------------------------

def parse_since(since: str) -> int:
    """把 24h / 7d / 30m 转成秒。"""
    m = re.fullmatch(r"\s*(\d+)\s*([smhd])\s*", since)
    if not m:
        raise argparse.ArgumentTypeError(f"--since 不识别: {since!r}，示例：24h / 7d / 30m")
    n, unit = int(m.group(1)), m.group(2)
    return n * {"s": 1, "m": 60, "h": 3600, "d": 86400}[unit]


def parse_iso(s: str) -> int:
    """ISO8601 / yyyy-mm-dd 转成 unix 秒（按本地时区）。"""
    for fmt in ("%Y-%m-%dT%H:%M:%S", "%Y-%m-%d %H:%M:%S", "%Y-%m-%d"):
        try:
            return int(datetime.strptime(s, fmt).timestamp())
        except ValueError:
            continue
    raise argparse.ArgumentTypeError(f"无法解析时间 {s!r}")


# ---------------------------------------------------------------------------
# 拉取主流程
# ---------------------------------------------------------------------------

def fetch_history(cli: str, chat_id: str, container_type: str,
                  start_sec: int, end_sec: int, user_token: str | None) -> tuple[list[dict], dict[str, str]]:
    """翻页拉主消息，按 ByCreateTimeAsc 升序。返回 (items, sender_names)。"""
    all_items: list[dict] = []
    sender_names: dict[str, str] = {}
    page_token = ""
    for round_i in range(1, 100):  # 安全上限
        args = [
            "msg", "history",
            "--container-id", chat_id,
            "--container-id-type", container_type,
            "--page-size", "50",
            "--start-time", str(start_sec),
            "--end-time", str(end_sec),
            "--sort-type", "ByCreateTimeAsc",
            # v1.27.1+: msg history 默认会自动展开线程，本脚本自己有独立的
            # fetch_thread 阶段（可输出 threads.json + index.md 等多文件结构），
            # 显式关闭避免双拉浪费 API quota。
            "--expand-threads=false",
            "-o", "json",
        ]
        if page_token:
            args += ["--page-token", page_token]
        d = run_cli_json(cli, args, user_token)
        if not d:
            break
        items = d.get("items") or []
        all_items.extend(items)
        sn = d.get("sender_names") or {}
        sender_names.update(sn)
        has_more = d.get("has_more", False)
        page_token = d.get("page_token") or ""
        print(f"  history 第 {round_i} 页: +{len(items)} 条, 累计 {len(all_items)}, has_more={has_more}")
        if not has_more or not page_token:
            break
    return all_items, sender_names


def fetch_thread(cli: str, thread_id: str, user_token: str | None) -> tuple[list[dict], dict[str, str]]:
    """拉单个话题的全部回复。注意：thread-messages 不接受 -o json（默认就是 JSON），
    返回字段是 PascalCase。"""
    msgs: list[dict] = []
    sender_names: dict[str, str] = {}
    page_token = ""
    for _ in range(50):
        args = ["msg", "thread-messages", thread_id, "--page-size", "50", "--sort", "ByCreateTimeAsc"]
        if page_token:
            args += ["--page-token", page_token]
        d = run_cli_json(cli, args, user_token)
        if not d:
            break
        # 兼容 PascalCase（thread-messages） 和 snake_case（万一某天 CLI 统一了）
        items = d.get("items") or d.get("Items") or []
        msgs.extend(items)
        sn = d.get("sender_names") or d.get("SenderNames") or {}
        sender_names.update(sn)
        has_more = d.get("has_more") if "has_more" in d else d.get("HasMore", False)
        page_token = d.get("page_token") or d.get("PageToken") or ""
        if not has_more or not page_token:
            break
    return msgs, sender_names


# ---------------------------------------------------------------------------
# 名字反解三级策略：mentions > sender_names > user info
# ---------------------------------------------------------------------------

def collect_mention_names(items: list[dict]) -> dict[str, str]:
    """从消息的 mentions 字段直接拿 open_id → name 映射。
    这是最便宜的反解来源，且不受外部租户隔离限制。"""
    names: dict[str, str] = {}
    for it in items:
        for m in (it.get("mentions") or []):
            if m.get("id_type") == "open_id" and m.get("id") and m.get("name"):
                names[m["id"]] = m["name"]
    return names


def resolve_with_user_info(cli: str, open_ids: list[str], user_token: str | None) -> dict[str, str]:
    """对剩余无名字的 open_id 调 user info；外部用户会 41050，静默跳过即可。"""
    out: dict[str, str] = {}
    for oid in open_ids:
        d = run_cli_json(cli, ["user", "info", oid, "-o", "json"], user_token)
        if isinstance(d, dict):
            n = d.get("name") or (d.get("user") or {}).get("name")
            if n:
                out[oid] = n
    return out


def resolve_bot_app_ids(items: list[dict], default_bot_name: str) -> dict[str, str]:
    """sender.id_type=app_id 时 id 是 cli_xxx，与 sender_names 中的 ou_xxx bot 不互通。
    一律映射成调用方提供的 default_bot_name（通常就是群里那个 bot 的名字）。"""
    out: dict[str, str] = {}
    for it in items:
        s = it.get("sender") or {}
        if s.get("id_type") == "app_id" and s.get("id"):
            out.setdefault(s["id"], default_bot_name)
    return out


# ---------------------------------------------------------------------------
# 渲染层
# ---------------------------------------------------------------------------

def parse_content(raw: Any) -> Any:
    """body.content 大多数是 JSON 字符串，但撤回消息是纯字符串 'This message was recalled'。"""
    if not isinstance(raw, str):
        return raw
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return raw  # 撤回 / 异常消息直接返回字符串


def render_system(c: dict) -> str:
    """system 消息的 template 占位符 {from_user} / {to_chatters} 等需要从同一对象其他字段填充。"""
    if not isinstance(c, dict):
        return str(c)
    tpl = c.get("template", "")
    for k, v in c.items():
        if k in ("template", "divider_text"):
            continue
        if isinstance(v, list):
            tpl = tpl.replace("{" + k + "}", ", ".join(str(x) for x in v))
        elif isinstance(v, str):
            tpl = tpl.replace("{" + k + "}", v)
    return tpl.strip()


def render_post(c: dict, names: dict[str, str]) -> str:
    """post content 实战中观察到两种结构：
    1) {"zh_cn": {"title": ..., "content": [[...]]}} — OpenAPI 文档示例
    2) {"title": ..., "content": [[...]]}            — IM 实际下发常用
    两种都要兼容。"""
    if isinstance(c, dict) and ("zh_cn" in c or "en_us" in c):
        block = c.get("zh_cn") or c.get("en_us") or {}
    else:
        block = c if isinstance(c, dict) else {}
    title = block.get("title", "")
    paragraphs: list[str] = []
    for line in block.get("content", []) or []:
        parts: list[str] = []
        for seg in line or []:
            if not isinstance(seg, dict):
                continue
            tag = seg.get("tag")
            if tag in ("text", "md"):
                parts.append(seg.get("text", ""))
            elif tag == "a":
                parts.append(f"[{seg.get('text','')}]({seg.get('href','')})")
            elif tag == "at":
                uid = seg.get("user_id", "")
                nm = seg.get("user_name") or names.get(uid) or uid
                parts.append("@" + nm)
            elif tag == "img":
                parts.append("[图片]")
            elif tag == "media":
                parts.append("[视频]")
            elif tag == "code_block":
                parts.append("```\n" + seg.get("text", "") + "\n```")
            elif tag == "hr":
                parts.append("---")
            else:
                parts.append(f"[{tag}]")
        paragraphs.append("".join(parts))
    return ((title + "\n") if title else "") + "\n".join(paragraphs)


def render_card_elements(els: list[dict]) -> list[str]:
    """递归处理卡片 body.elements，覆盖 markdown / div / column_set / form /
    collapsible_panel / action / button / img / hr / note。"""
    lines: list[str] = []
    for el in els or []:
        if not isinstance(el, dict):
            continue
        tag = el.get("tag")
        if tag in ("markdown", "div", "plain_text"):
            txt = el.get("content") or (el.get("text") or {}).get("content") or ""
            if txt:
                lines.append(txt)
            for f in (el.get("fields") or []):
                t = (f.get("text") or {}).get("content") or ""
                if t:
                    lines.append("· " + t)
        elif tag == "column_set":
            for col in el.get("columns", []) or []:
                lines.extend(render_card_elements(col.get("elements", [])))
        elif tag == "form":
            lines.extend(render_card_elements(el.get("elements", [])))
        elif tag == "collapsible_panel":
            hdr = (el.get("header") or {}).get("title", {}).get("content", "")
            if hdr:
                lines.append(f"[折叠面板] {hdr}")
            lines.extend(render_card_elements(el.get("elements", [])))
        elif tag == "action":
            for act in el.get("actions") or []:
                t = (act.get("text") or {}).get("content") or ""
                url = act.get("url") or (act.get("multi_url") or {}).get("url") or ""
                if t or url:
                    lines.append(f"[按钮] {t} {('→ ' + url) if url else ''}".strip())
        elif tag == "button":
            t = (el.get("text") or {}).get("content") or ""
            url = el.get("url") or (el.get("multi_url") or {}).get("url") or ""
            if t or url:
                lines.append(f"[按钮] {t} {('→ ' + url) if url else ''}".strip())
        elif tag == "img":
            alt = (el.get("alt") or {}).get("content") or ""
            lines.append(f"[图片] {alt}".strip())
        elif tag == "hr":
            lines.append("---")
        elif tag == "note":
            sub: list[str] = []
            for sub_el in el.get("elements") or []:
                t = sub_el.get("content") or (sub_el.get("text") or {}).get("content") or ""
                if t:
                    sub.append(t)
            if sub:
                lines.append("📌 " + " ".join(sub))
        else:
            t = el.get("content")
            if t:
                lines.append(t)
    return lines


def render_card(c: Any, card_texts: Any) -> str:
    """interactive 卡片有四种来源：v2 schema / v1 elements / template_id / card_id。"""
    if not isinstance(c, dict):
        return f"[卡片] {str(c)[:100]}"
    out: list[str] = []
    if c.get("schema") == "2.0":
        hdr = c.get("header") or {}
        title = (hdr.get("title") or {}).get("content", "")
        subtitle = (hdr.get("subtitle") or {}).get("content", "")
        template = hdr.get("template", "")
        if title or subtitle:
            head = f"【卡片|{template}】{title}" if template else f"【卡片】{title}"
            if subtitle:
                head += f" — {subtitle}"
            out.append(head)
        out.extend(render_card_elements((c.get("body") or {}).get("elements", [])))
    elif c.get("type") == "template":
        out.append(f"[卡片(template_id={c.get('data',{}).get('template_id','')})]")
    elif c.get("type") == "card":
        out.append(f"[卡片(card_id={c.get('data',{}).get('card_id','')})]")
    else:
        # 老版 v1：header.title.content + elements[]
        hdr = c.get("header") or {}
        title = (hdr.get("title") or {}).get("content", "")
        if title:
            out.append(f"【卡片】{title}")
        out.extend(render_card_elements(c.get("elements") or []))
    if not out and card_texts:
        # API 已经提取的纯文本兜底
        out.append("\n".join(card_texts) if isinstance(card_texts, list) else str(card_texts))
    return "\n".join(x for x in out if x) or "[卡片(空)]"


def ts(ms: str | int) -> str:
    return datetime.fromtimestamp(int(ms) / 1000).strftime("%Y-%m-%d %H:%M:%S")


def render_message(item: dict, names: dict[str, str], indent: str = "") -> list[str]:
    t = ts(item["create_time"])
    mtype = item["msg_type"]
    sid = (item.get("sender") or {}).get("id", "")
    sname = names.get(sid, sid or "?")
    raw = (item.get("body") or {}).get("content", "")
    c = parse_content(raw)
    card_texts = (item.get("body") or {}).get("card_texts") if isinstance(item.get("body"), dict) else None

    if isinstance(c, str):
        body = c  # 撤回消息 / 纯字符串
    elif mtype == "system":
        body = "[系统] " + render_system(c)
    elif mtype == "text":
        text = c.get("text", "")
        # mentions 里的 key 形如 @_user_1，替换成真实姓名
        for m in (item.get("mentions") or []):
            key = m.get("key", "")
            name = m.get("name", "")
            if key and name:
                text = text.replace(key, "@" + name)
        body = text
    elif mtype == "post":
        body = render_post(c, names) or "[富文本(空)]"
    elif mtype == "interactive":
        body = render_card(c, card_texts)
    elif mtype == "image":
        body = f"[图片] image_key={c.get('image_key','')}"
    elif mtype == "file":
        body = "[文件] " + (c.get("file_name") or c.get("file_key", ""))
    elif mtype == "audio":
        body = "[语音]"
    elif mtype == "media":
        body = "[视频]"
    elif mtype == "share_chat":
        body = f"[群名片] chat_id={c.get('chat_id','')}"
    elif mtype == "share_user":
        body = f"[个人名片] user_id={c.get('user_id','')}"
    else:
        body = f"[{mtype}] {json.dumps(c, ensure_ascii=False)[:200]}"

    body = body.strip()
    label = "" if mtype == "system" else f" {sname}"
    head = f"{indent}[{t}]{label}"
    lines = body.splitlines() or [""]
    out = [f"{head} | {lines[0]}"]
    for line in lines[1:]:
        out.append(f"{indent}    {line}")
    return out


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main() -> int:
    p = argparse.ArgumentParser(
        description="拉取飞书群聊（含话题）的消息记录，输出 JSON + 渲染后的时间线",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__.split("用法")[1].split("输出")[0] if "用法" in (__doc__ or "") else "",
    )
    p.add_argument("chat_id", help="群 chat_id（oc_xxx）或话题 thread_id（omt_xxx）")
    p.add_argument("--container-type", default="chat", choices=["chat", "thread"],
                   help="容器类型，默认 chat；当 chat_id 是 omt_xxx 时改为 thread")
    p.add_argument("--since", help="时间窗口，例如 24h / 7d / 30m。与 --start/--end 互斥")
    p.add_argument("--start", help="起始时间 ISO8601（如 2026-05-21T00:00:00），与 --since 互斥")
    p.add_argument("--end", help="结束时间 ISO8601，默认 now")
    p.add_argument("--output-dir", default="/tmp/lark_chat", help="输出目录，默认 /tmp/lark_chat")
    p.add_argument("--no-thread", action="store_true", help="不展开话题回复")
    p.add_argument("--no-name-resolve", action="store_true", help="不调用 user info 反解名字（mentions 兜底仍生效）")
    p.add_argument("--bot-name", default="Bot", help="bot app_id 的展示名字，默认 Bot")
    p.add_argument("--cli", help="feishu-cli 二进制路径，默认 PATH 或项目根目录")
    p.add_argument("--user-access-token", help="显式 User Access Token（默认走登录态）")
    p.add_argument("-v", "--verbose", action="store_true", help="打印 CLI 调用错误")
    args = p.parse_args()

    if args.verbose:
        os.environ["FETCH_CHAT_VERBOSE"] = "1"

    cli = find_cli(args.cli)
    out_dir = Path(args.output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    user_token = args.user_access_token

    now_sec = int(time.time())
    if args.start or args.end:
        start_sec = parse_iso(args.start) if args.start else now_sec - 86400
        end_sec = parse_iso(args.end) if args.end else now_sec
    else:
        delta = parse_since(args.since or "24h")
        start_sec = now_sec - delta
        end_sec = now_sec

    print(f"[1/4] 拉取主消息 chat_id={args.chat_id} "
          f"窗口=[{datetime.fromtimestamp(start_sec)} ~ {datetime.fromtimestamp(end_sec)}]")
    items, sender_names = fetch_history(cli, args.chat_id, args.container_type,
                                        start_sec, end_sec, user_token)
    print(f"[1/4] done: {len(items)} 条主消息")
    (out_dir / "history.json").write_text(
        json.dumps({"items": items, "sender_names": sender_names}, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )

    # 展开话题
    threads: dict[str, list[dict]] = {}
    if not args.no_thread:
        thread_ids = sorted({it.get("thread_id") for it in items if it.get("thread_id")})
        print(f"[2/4] 展开 {len(thread_ids)} 个话题")
        for i, tid in enumerate(thread_ids, 1):
            msgs, sn_t = fetch_thread(cli, tid, user_token)
            sender_names.update(sn_t)
            threads[tid] = msgs
            if i % 5 == 0 or i == len(thread_ids):
                print(f"  {i}/{len(thread_ids)} threads")
        print(f"[2/4] done: {sum(len(v) for v in threads.values())} 条线程消息")
    (out_dir / "threads.json").write_text(
        json.dumps(threads, ensure_ascii=False, indent=2), encoding="utf-8",
    )

    # 名字反解：mentions > sender_names > user info > bot 默认名
    all_msgs = list(items) + [m for v in threads.values() for m in v]
    names = dict(sender_names)
    names.update(collect_mention_names(all_msgs))

    open_ids = {(it.get("sender") or {}).get("id", "") for it in all_msgs}
    open_ids = {x for x in open_ids if x.startswith("ou_")}
    remaining = [oid for oid in sorted(open_ids) if oid not in names]
    if remaining and not args.no_name_resolve:
        print(f"[3/4] 反解 {len(remaining)} 个 open_id（外部租户会 41050 静默跳过）")
        names.update(resolve_with_user_info(cli, remaining, user_token))
    names.update(resolve_bot_app_ids(all_msgs, args.bot_name))
    print(f"[3/4] done: 已知名字 {len(names)} 个")
    (out_dir / "names.json").write_text(
        json.dumps(names, ensure_ascii=False, indent=2), encoding="utf-8",
    )

    # 渲染时间线：主消息升序 + 每条主消息后跟随其线程回复（除自身）
    print("[4/4] 渲染时间线")
    out: list[str] = []
    out.append(f"# chat_id={args.chat_id}  container={args.container_type}")
    out.append(f"# 窗口: {datetime.fromtimestamp(start_sec)} ~ {datetime.fromtimestamp(end_sec)}")
    out.append(f"# 主消息 {len(items)} 条 / 线程 {len(threads)} 个 / 线程消息 {sum(len(v) for v in threads.values())} 条")
    out.append("")
    for it in sorted(items, key=lambda x: int(x["create_time"])):
        out.extend(render_message(it, names))
        tid = it.get("thread_id")
        if tid and tid in threads:
            replies = [m for m in threads[tid] if m.get("message_id") != it.get("message_id")]
            replies.sort(key=lambda x: int(x["create_time"]))
            for r in replies:
                out.extend(render_message(r, names, indent="    └─ "))
    (out_dir / "timeline.txt").write_text("\n".join(out) + "\n", encoding="utf-8")
    print(f"[4/4] done: {out_dir/'timeline.txt'} ({len(out)} 行)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
