#!/usr/bin/env python3
"""Embed a self-contained HTML file into a Feishu Docx as an HTML Box widget.

Backend: `feishu-cli` only (the `api` passthrough + docx create endpoint). The
flow mirrors the standard HTML Box embedding contract:

  1. Create the doc (or reuse --doc-token).
  2. Insert an HTML code block: block_type=14, code.style.language=24.
  3. Insert an HTML Box widget: block_type=40, with the HTML prefilled into
     add_ons.record so the sandbox iframe renders immediately.
  4. By default delete the source code block — the widget keeps its own record,
     so the iframe still renders but the doc isn't bloated with raw HTML.

Everything runs under a single identity (--as, default "user") so the new doc
and the inserted blocks share an owner with edit permission. Run
`feishu-cli auth login` first when using --as user.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path

HTML_LANGUAGE = 24
HTML_BOX_COMPONENT_TYPE_ID = "blk_6900429af84180025ce76527"


class FeishuCLIError(RuntimeError):
    pass


def run_json(cli: str, api_args: list[str], *, dry_run: bool) -> dict:
    """Invoke `feishu-cli api ...` and parse its stdout as the Feishu envelope."""
    cmd = [cli, "api", *api_args]
    if dry_run:
        print("[dry-run]", " ".join(cmd), file=sys.stderr)
        return {"code": 0, "data": {}}
    proc = subprocess.run(cmd, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if proc.returncode != 0:
        raise FeishuCLIError(
            f"command failed ({proc.returncode}): {' '.join(cmd)}\n"
            f"stdout:\n{proc.stdout}\n\nstderr:\n{proc.stderr}"
        )
    try:
        env = json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        raise FeishuCLIError(
            f"feishu-cli did not return JSON: {' '.join(cmd)}\n{proc.stdout}"
        ) from exc
    if isinstance(env, dict) and env.get("code") not in (0, None):
        raise FeishuCLIError(
            f"Feishu business error code={env.get('code')} msg={env.get('msg')}\n"
            f"request: {' '.join(cmd)}"
        )
    return env


def api(cli: str, method: str, path: str, *, identity: str, data: dict | None, dry_run: bool) -> dict:
    args = [method, path, "--as", identity]
    if data is not None:
        args += ["--data", json.dumps(data, ensure_ascii=False)]
    return run_json(cli, args, dry_run=dry_run)


def create_doc(cli: str, title: str, folder_token: str | None, identity: str, dry_run: bool) -> str:
    body: dict = {"title": title}
    if folder_token:
        body["folder_token"] = folder_token
    resp = api(cli, "POST", "/open-apis/docx/v1/documents", identity=identity, data=body, dry_run=dry_run)
    if dry_run:
        return "DRYRUN_DOC_TOKEN"
    doc_token = (((resp.get("data") or {}).get("document")) or {}).get("document_id")
    if not doc_token:
        raise FeishuCLIError(f"failed to create doc: {json.dumps(resp, ensure_ascii=False)}")
    return doc_token


def children_path(doc_token: str) -> str:
    return f"/open-apis/docx/v1/documents/{doc_token}/blocks/{doc_token}/children"


def insert_code_block(cli: str, doc_token: str, html: str, identity: str, dry_run: bool) -> str:
    payload = {
        "children": [
            {
                "block_type": 14,
                "code": {
                    "style": {"language": HTML_LANGUAGE, "wrap": True},
                    "elements": [{"text_run": {"content": html}}],
                },
            }
        ],
        "index": -1,
    }
    resp = api(cli, "POST", children_path(doc_token), identity=identity, data=payload, dry_run=dry_run)
    if dry_run:
        return "DRYRUN_CODE_BLOCK_ID"
    child = (resp.get("data") or {}).get("children", [{}])[0]
    block_id = child.get("block_id")
    language = (((child.get("code") or {}).get("style")) or {}).get("language")
    if language != HTML_LANGUAGE or not block_id:
        raise FeishuCLIError(
            f"HTML code block was not created correctly (language={language}): "
            f"{json.dumps(resp, ensure_ascii=False)}"
        )
    return block_id


def insert_html_box(cli: str, doc_token: str, html: str, identity: str, dry_run: bool) -> str:
    record = json.dumps({"html": html}, ensure_ascii=False)
    payload = {
        "children": [
            {
                "block_type": 40,
                "add_ons": {
                    "component_id": "",
                    "component_type_id": HTML_BOX_COMPONENT_TYPE_ID,
                    "record": record,
                },
            }
        ],
        "index": -1,
    }
    resp = api(cli, "POST", children_path(doc_token), identity=identity, data=payload, dry_run=dry_run)
    if dry_run:
        return "DRYRUN_HTML_BOX_ID"
    child = (resp.get("data") or {}).get("children", [{}])[0]
    block_id = child.get("block_id")
    persisted_record = (child.get("add_ons") or {}).get("record") or ""
    try:
        persisted_html = json.loads(persisted_record).get("html")
    except json.JSONDecodeError:
        persisted_html = None
    if not block_id or persisted_html != html:
        raise FeishuCLIError(
            f"HTML Box record was not persisted correctly: {json.dumps(resp, ensure_ascii=False)}"
        )
    return block_id


def delete_root_child(cli: str, doc_token: str, block_id: str, identity: str, dry_run: bool) -> bool:
    if dry_run:
        return True
    resp = api(
        cli, "GET", f"/open-apis/docx/v1/documents/{doc_token}/blocks/{doc_token}",
        identity=identity, data=None, dry_run=dry_run,
    )
    children = (((resp.get("data") or {}).get("block")) or {}).get("children", [])
    try:
        idx = children.index(block_id)
    except ValueError:
        return False
    api(
        cli, "DELETE", f"/open-apis/docx/v1/documents/{doc_token}/blocks/{doc_token}/children/batch_delete",
        identity=identity, data={"start_index": idx, "end_index": idx + 1}, dry_run=dry_run,
    )
    return True


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--html", required=True, help="Self-contained HTML file to embed")
    parser.add_argument("--title", help="Create a new doc with this title")
    parser.add_argument("--doc-token", help="Append into an existing Docx (document_id)")
    parser.add_argument("--folder-token", help="Optional parent folder when creating a new doc")
    parser.add_argument("--as", dest="identity", default="user", choices=["user", "bot", "auto"],
                        help="Identity for every feishu-cli api call (default: user)")
    parser.add_argument("--keep-source", action="store_true",
                        help="Keep the HTML source code block visible in the doc")
    parser.add_argument("--feishu-cli", default="feishu-cli", help="Path to the feishu-cli binary")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print the feishu-cli commands without calling the API")
    args = parser.parse_args(argv)

    html_path = Path(args.html)
    if not html_path.is_file():
        raise SystemExit(f"HTML file not found: {html_path}")
    if not args.title and not args.doc_token:
        raise SystemExit("need --title for a new doc or --doc-token to append into an existing doc")

    html = html_path.read_text(encoding="utf-8")
    cli = args.feishu_cli

    try:
        doc_token = args.doc_token or create_doc(cli, args.title, args.folder_token, args.identity, args.dry_run)
        code_block_id = insert_code_block(cli, doc_token, html, args.identity, args.dry_run)
        html_box_block_id = insert_html_box(cli, doc_token, html, args.identity, args.dry_run)
        source_deleted = False
        if not args.keep_source:
            source_deleted = delete_root_child(cli, doc_token, code_block_id, args.identity, args.dry_run)
    except FeishuCLIError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    print(json.dumps({
        "ok": True,
        "doc_token": doc_token,
        "doc_url": f"https://feishu.cn/docx/{doc_token}",
        "code_block_id": code_block_id,
        "html_box_block_id": html_box_block_id,
        "code_language": HTML_LANGUAGE,
        "source_deleted": source_deleted,
        "dry_run": args.dry_run,
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
