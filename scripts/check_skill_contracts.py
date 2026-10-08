#!/usr/bin/env python3
"""用当前编译产物执行可重复的离线 Skill 契约测试（dry-run / 本地 HTTP mock）。"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[1]
DOC_FIXTURES = ROOT / "skills/feishu-cli-docs/evals/fixtures"
FIELD_FIXTURE = ROOT / "skills/feishu-cli-data/evals/fixtures/select-field.json"


def run_contracts(binary: Path) -> list[dict]:
    requests: list[dict] = []

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def handle_api(self):
            raw = self.rfile.read(int(self.headers.get("Content-Length", 0)))
            body = json.loads(raw) if raw else None
            path = urlsplit(self.path).path
            requests.append({"method": self.command, "path": path, "body": body})
            status = 200
            if path == "/open-apis/auth/v3/tenant_access_token/internal":
                response = {"code": 0, "tenant_access_token": "t-fixture", "expire": 7200}
            elif path == "/open-apis/docs_ai/v1/documents/doccn_fixture":
                response = {"code": 0, "data": {"result": "success", "revision_id": 1}}
            elif path == "/open-apis/attendance/v1/user_tasks/query":
                response = {"code": 0, "data": {"user_task_results": []}}
            elif path == "/open-apis/base/v3/bases/bascn_fixture/tables/tbl_fixture/fields":
                response = {"code": 0, "data": {"field": {"id": "fld_fixture", **body}}}
            elif path == "/open-apis/drive/v1/metas/batch_query":
                response = {"code": 0, "data": {"metas": [{"doc_token": "boxcn_fixture", "doc_type": "file", "title": ""}]}}
            else:
                status = 400
                response = {"code": 400, "msg": "unexpected offline contract request"}
            encoded = json.dumps(response).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

        do_POST = handle_api
        do_PUT = handle_api
        do_GET = handle_api

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    results: list[dict] = []
    try:
        with tempfile.TemporaryDirectory(prefix="feishu-skill-contract-") as tmp:
            directory = Path(tmp)
            config = directory / "config.yaml"
            config.write_text("app_id: cli_fixture\napp_secret: fixture\n", encoding="utf-8")
            env = {key: value for key, value in os.environ.items() if not key.startswith("FEISHU_")}
            env.update({
                "FEISHU_APP_ID": "cli_fixture", "FEISHU_APP_SECRET": "fixture",
                "FEISHU_USER_ACCESS_TOKEN": "u-fixture", "FEISHU_CLI_REMOTE_META": "off",
                "FEISHU_BASE_URL": f"http://127.0.0.1:{server.server_port}",
                "FEISHU_ALLOW_CUSTOM_BASE_URL": "true", "FEISHU_ALLOW_INSECURE_HTTP": "true",
                "NO_PROXY": "127.0.0.1,localhost", "no_proxy": "127.0.0.1,localhost",
            })

            def run(args: list[str], expected: int = 0, use_config: bool = True):
                command = [str(binary), *(["--config", str(config)] if use_config else []), *args]
                proc = subprocess.run(command, cwd=directory, env=env, capture_output=True, text=True, timeout=20, check=False)
                if proc.returncode != expected:
                    raise AssertionError(f"exit={proc.returncode}, expected={expected}; {proc.stderr.strip()}; {proc.stdout.strip()}")
                return proc

            def no_requests():
                assert not requests, f"本地验证/dry-run 不应发请求: {requests}"

            def content_update_rejects_uploads():
                proc = run(["doc", "content-update", "doccn_fixture", "--mode", "append", "--markdown", "测试", "--upload-images"], 2)
                assert "不支持 --upload-images" in proc.stderr, proc.stderr
                no_requests()
                proc = run(["doc", "content-update", "doccn_fixture", "--mode", "append", "--markdown-file", str(DOC_FIXTURES / "local-image.md")], 2)
                assert "本地" in proc.stderr, proc.stderr
                no_requests()

            def content_update_remote_image_payload():
                fixture = DOC_FIXTURES / "remote-image.md"
                run(["doc", "content-update", "doccn_fixture", "--mode", "append", "--markdown-file", str(fixture), "--user-access-token", "u-fixture"])
                assert requests == [{"method": "PUT", "path": "/open-apis/docs_ai/v1/documents/doccn_fixture", "body": {
                    "format": "markdown", "command": "block_insert_after", "block_id": "-1", "content": fixture.read_text(encoding="utf-8"), "revision_id": -1,
                }}], requests

            def drive_export_bot_dry_run():
                proc = run(["drive", "export", "--token", "doccn_fixture", "--doc-type", "docx", "--file-extension", "pdf", "--as", "bot", "--dry-run"])
                plan = json.loads(proc.stdout)
                assert "export_tasks" in json.dumps(plan), plan
                no_requests()

            def api_bot_identity_overrides_user_token():
                proc = run(["api", "GET", "/open-apis/authen/v1/user_info", "--as", "bot", "--user-access-token", "u-fixture", "--dry-run"])
                plan = json.loads(proc.stdout)
                assert plan["will_use_user_tok"] is False and plan["supported_tokens"] == ["tenant_access_token"], plan
                no_requests()

            def attendance_dates():
                prefix = ["attendance", "user-task", "query", "--employee-type", "employee_id", "--user-ids", "employee_fixture", "--as", "user", "--user-access-token", "u-fixture"]
                run([*prefix, "--start", "2026-05-01", "--end", "20260502", "-o", "json"])
                assert len(requests) == 1, requests
                request = requests[0]
                assert request["path"] == "/open-apis/attendance/v1/user_tasks/query", request
                assert request["body"]["check_date_from"] == 20260501 and request["body"]["check_date_to"] == 20260502, request
                requests.clear()
                run([*prefix, "--start", "2026-05-01T00:00:00+08:00", "--end", "2026-05-02"], 1)
                no_requests()

            def field_payload():
                # field create 的 --config 是字段 JSON，故该命令用环境变量注入 mock 端点与占位凭证。
                run(["bitable", "field", "create", "--base-token", "bascn_fixture", "--table-id", "tbl_fixture", "--config-file", str(FIELD_FIXTURE), "--as", "user", "--user-access-token", "u-fixture"], use_config=False)
                expected = json.loads(FIELD_FIXTURE.read_text(encoding="utf-8"))
                assert expected["type"] == "select" and isinstance(expected["options"], list), expected
                assert "field" not in expected and "property" not in expected, expected
                assert len(requests) == 1 and requests[0]["body"] == expected, requests

            def owner_configuration():
                for key, expected in (("owner_email", "user@example.com"), ("transfer_ownership", "false")):
                    proc = run(["--config", str(DOC_FIXTURES / "owner-config.yaml"), "config", "get", key], use_config=False)
                    assert expected in proc.stdout, proc.stdout
                no_requests()

            def large_xlsx_keeps_sheet():
                source = directory / "large.xlsx"
                with source.open("wb") as stream:
                    stream.truncate(21 * 1024 * 1024)
                proc = run(["drive", "import", "--file", str(source), "--type", "sheet", "--as", "bot", "--dry-run"])
                plan = json.loads(proc.stdout)
                encoded = json.dumps(plan)
                assert "upload_prepare" in encoded and '"sheet"' in encoded and "bitable" not in encoded, plan
                no_requests()

            def markdown_empty_name_rejects_upload():
                run(["markdown", "overwrite", "--file-token", "boxcn_fixture", "--content", "更新", "--as", "bot"], 1)
                business = [r for r in requests if "/auth/" not in r["path"]]
                assert len(business) == 1 and business[0]["path"] == "/open-apis/drive/v1/metas/batch_query", business

            cases = [content_update_rejects_uploads, content_update_remote_image_payload,
                     drive_export_bot_dry_run, api_bot_identity_overrides_user_token, attendance_dates, field_payload,
                     owner_configuration, large_xlsx_keeps_sheet, markdown_empty_name_rejects_upload]
            for case in cases:
                requests.clear()
                try:
                    case()
                except (AssertionError, OSError, ValueError, subprocess.TimeoutExpired) as exc:
                    results.append({"name": case.__name__, "passed": False, "error": str(exc)})
                else:
                    results.append({"name": case.__name__, "passed": True})
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
    return results


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path, help="当前源码构建的 feishu-cli")
    parser.add_argument("--json-report", type=Path, help="保存离线行为验证结果")
    args = parser.parse_args()
    binary = args.binary.resolve()
    if not binary.is_file():
        parser.error(f"二进制不存在: {binary}")
    results = run_contracts(binary)
    for result in results:
        print(f"{'PASS' if result['passed'] else 'FAIL'} {result['name']}" + (f": {result['error']}" if not result["passed"] else ""))
    if args.json_report:
        args.json_report.write_text(json.dumps(results, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"离线二进制契约: {sum(row['passed'] for row in results)}/{len(results)} 通过（不调用飞书线上 API，不代表线上权限/业务成功率）。")
    return int(any(not row["passed"] for row in results))


if __name__ == "__main__":
    raise SystemExit(main())
