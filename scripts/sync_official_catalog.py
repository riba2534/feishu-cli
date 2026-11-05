#!/usr/bin/env -S uv run python3 -I
"""从官方 lark-cli 仓库的内嵌 catalog 重生成本项目 internal/registry/meta_data.json。

用法:
  scripts/sync_official_catalog.py <官方仓库路径> [--output internal/registry/meta_data.json]
      [--supplements internal/registry/local_supplements.json] [--allow-remove svc.res.method ...]

流程:
  1. 读取 <官方仓库>/internal/registry/catalog/manifest.json，逐个 service shard 校验 size 与 sha256；
  2. 合并为 {"services": [...], "version": ..., "source": {...}}，service 按 name 排序、所有键排序，保证 diff 稳定；
  3. 与输出路径上的旧文件做 method 级 diff（增、删，以及 path/httpMethod/accessTokens/scopes 变化）；
  4. 出现删除且不在 --allow-remove 白名单内，或 local_supplements 白名单中的方法在新 catalog 里缺失时，
     不写文件并以非零码退出。

只用标准库。顶层 version 默认沿用旧文件（缺省 1.0.0）：运行时 remote overlay 会拒绝比内嵌版本更旧的
cache，而官方 meta 端点的顶层 version 长期恒为 1.0.0，随意抬高会让联网 overlay 失效。
"""

import argparse
import hashlib
import json
import os
import subprocess
import sys

MANIFEST_REL = os.path.join("internal", "registry", "catalog", "manifest.json")
DEFAULT_VERSION = "1.0.0"


def fail(msg):
    print("错误: " + msg, file=sys.stderr)
    sys.exit(1)


def load_official(repo):
    manifest_path = os.path.join(repo, MANIFEST_REL)
    try:
        with open(manifest_path, "rb") as f:
            manifest = json.loads(f.read())
    except (OSError, ValueError) as e:
        fail("读取官方 manifest 失败: %s (%s)" % (manifest_path, e))
    if manifest.get("schema_version") != 1:
        fail("不支持的 manifest schema_version: %r（脚本按 1 编写，请先核对格式）" % manifest.get("schema_version"))
    catalog_dir = os.path.realpath(os.path.dirname(manifest_path))
    services = []
    seen = set()
    for entry in manifest.get("services") or []:
        name, rel = entry.get("name"), entry.get("file")
        if not name or not rel:
            fail("manifest 条目缺少 name/file: %r" % entry)
        if name in seen:
            fail("manifest 中 service 重复: %s" % name)
        seen.add(name)
        path = os.path.realpath(os.path.join(catalog_dir, rel))
        if os.path.commonpath([catalog_dir, path]) != catalog_dir:
            fail("manifest 文件路径越界: %s" % rel)
        with open(path, "rb") as f:
            raw = f.read()
        if len(raw) != entry.get("size"):
            fail("%s size 不符: manifest=%s 实际=%d" % (rel, entry.get("size"), len(raw)))
        digest = hashlib.sha256(raw).hexdigest()
        if digest != entry.get("sha256"):
            fail("%s sha256 不符: manifest=%s 实际=%s" % (rel, entry.get("sha256"), digest))
        svc = json.loads(raw)
        if svc.get("name") != name:
            fail("%s 的 name=%r 与 manifest 的 %r 不一致" % (rel, svc.get("name"), name))
        services.append(svc)
    if not services:
        fail("官方 manifest 没有任何 service")
    services.sort(key=lambda s: s["name"])
    return manifest, services


def git_info(repo):
    def run(*args):
        try:
            out = subprocess.run(["git", "-C", repo] + list(args), capture_output=True, text=True, check=True)
            return out.stdout.strip()
        except (OSError, subprocess.CalledProcessError):
            return ""

    return run("rev-parse", "HEAD"), run("describe", "--tags", "--always"), run("status", "--porcelain", "--", MANIFEST_REL, os.path.dirname(MANIFEST_REL))


def walk(resources, prefix=()):
    for rname in sorted(resources or {}):
        res = resources[rname] or {}
        path = prefix + (rname,)
        for mname in sorted(res.get("methods") or {}):
            yield path, mname, res["methods"][mname]
        yield from walk(res.get("resources"), path)


def method_index(services):
    out = {}
    for svc in services:
        base = svc.get("servicePath", "")
        for path, mname, m in walk(svc.get("resources")):
            key = ".".join((svc["name"],) + path + (mname,))
            out[key] = {
                "path": base + "/" + (m.get("path") or ""),
                "httpMethod": m.get("httpMethod", ""),
                "accessTokens": list(m.get("accessTokens") or []),
                "scopes": list(m.get("scopes") or []),
            }
    return out


def describe_change(field, old, new):
    if isinstance(old, list) and sorted(old) == sorted(new):
        return "%s 仅顺序变化 %s -> %s" % (field, old, new)
    return "%s: %s -> %s" % (field, old, new)


def diff(old_services, new_services):
    old, new = method_index(old_services), method_index(new_services)
    added = sorted(set(new) - set(old))
    removed = sorted(set(old) - set(new))
    changed = {}
    for key in sorted(set(old) & set(new)):
        notes = [describe_change(f, old[key][f], new[key][f]) for f in ("path", "httpMethod", "accessTokens", "scopes") if old[key][f] != new[key][f]]
        if notes:
            changed[key] = notes
    return old, new, added, removed, changed


def supplement_ids(path):
    if not path or not os.path.exists(path):
        return []
    with open(path, "rb") as f:
        data = json.loads(f.read())
    ids = []
    for m in data.get("methods") or []:
        ids.append(".".join([m["service"]] + list(m["resources"]) + [m["method"]]))
    return ids


def main():
    ap = argparse.ArgumentParser(description="从官方 lark-cli catalog 重生成 meta_data.json")
    ap.add_argument("official_repo", help="官方 lark-cli 仓库路径（只读）")
    ap.add_argument("--output", default=os.path.join("internal", "registry", "meta_data.json"), help="输出路径（同时作为 diff 的旧版本）")
    ap.add_argument("--supplements", default=None, help="local_supplements.json 路径（默认与 --output 同目录）")
    ap.add_argument("--allow-remove", action="append", default=[], metavar="SVC.RES.METHOD", help="允许删除的方法，可重复")
    ap.add_argument("--version", default=None, help="顶层 version（默认沿用旧文件，缺省 %s）" % DEFAULT_VERSION)
    ap.add_argument("--dry-run", action="store_true", help="只输出 diff 报告，不写文件")
    args = ap.parse_args()

    _, services = load_official(args.official_repo)
    commit, describe, dirty = git_info(args.official_repo)

    old_services, old_version = [], None
    if os.path.exists(args.output):
        with open(args.output, "rb") as f:
            old = json.loads(f.read())
        old_services, old_version = old.get("services") or [], old.get("version")

    _, new_idx, added, removed, changed = diff(old_services, services)
    print("官方 catalog: %s (commit %s)%s" % (describe or "unknown", commit or "unknown", "，catalog 有未提交改动" if dirty else ""))
    print("service %d 个，method %d 个；新增 %d、删除 %d、变化 %d" % (len(services), len(new_idx), len(added), len(removed), len(changed)))
    for key in added:
        print("  + " + key)
    for key in removed:
        print("  - " + key + ("（已允许）" if key in args.allow_remove else ""))
    for key, notes in changed.items():
        for note in notes:
            print("  ~ %s %s" % (key, note))

    errors = []
    unexpected = [k for k in removed if k not in args.allow_remove]
    if unexpected:
        errors.append("以下方法将被删除且不在 --allow-remove 白名单: " + ", ".join(unexpected))
    sup_path = args.supplements or os.path.join(os.path.dirname(args.output) or ".", "local_supplements.json")
    missing = [k for k in supplement_ids(sup_path) if k not in new_idx]
    if missing:
        errors.append("local_supplements 中的方法在新 catalog 缺失: " + ", ".join(missing))
    if errors:
        for e in errors:
            print("错误: " + e, file=sys.stderr)
        sys.exit(1)
    if args.dry_run:
        return

    out = {
        "services": services,
        "source": {"repo": "larksuite/cli", "describe": describe, "commit": commit, "manifest": MANIFEST_REL.replace(os.sep, "/")},
        "version": args.version or old_version or DEFAULT_VERSION,
    }
    text = json.dumps(out, ensure_ascii=False, sort_keys=True, indent=1) + "\n"
    tmp = args.output + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        f.write(text)
    os.replace(tmp, args.output)
    print("已写入 %s（%d 字节）" % (args.output, len(text.encode("utf-8"))))


if __name__ == "__main__":
    main()
