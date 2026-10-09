"""校验器回归：畸形 YAML、示例参数和原有 8 正/8 负生成约定。"""

import json
import tempfile
import unittest
from pathlib import Path

from build_trigger_eval_set import build_eval_set
from check_skills import MANIFEST, check_explicit_group_prefixes, load_frontmatters, resolve_owner, validate_metadata
from skill_command_contracts import check_example, examples, parse_help


class FrontmatterTests(unittest.TestCase):
    def test_real_yaml_rejects_syntax_and_duplicate_keys(self):
        fixtures = {
            "valid": 'name: feishu-cli-test\ndescription: >-\n  第一行\n  第二行\nmetadata:\n  version: "1"\n',
            "syntax": "name: feishu-cli-test\ndescription: [unclosed\n",
            "duplicate": "name: feishu-cli-test\ndescription: first\ndescription: second\n",
            "nested_duplicate": "name: feishu-cli-test\ndescription: test\nmetadata:\n  version: one\n  version: two\n",
            "list": "- name\n- description\n",
            "typed": "name: 123\ndescription: false\n",
        }
        with tempfile.TemporaryDirectory() as tmp:
            paths = []
            for name, yaml in fixtures.items():
                path = Path(tmp) / (name + ".md")
                path.write_text("---\n" + yaml + "---\n# Body\n", encoding="utf-8")
                paths.append(path)
            parsed = load_frontmatters(paths)
            self.assertEqual(parsed[str(paths[0])]["metadata"]["description"], "第一行 第二行")
            self.assertEqual(validate_metadata(parsed[str(paths[0])]["metadata"], "feishu-cli-test"), [])
            for path in paths[1:5]:
                with self.subTest(path=path.name):
                    self.assertIn("error", parsed[str(path)])
            self.assertIs(parsed[str(paths[5])]["metadata"]["description"], False)
            self.assertTrue(validate_metadata(parsed[str(paths[5])]["metadata"], "feishu-cli-test"))

    def test_required_types_lengths_and_portable_fields(self):
        base = {"name": "feishu-cli-test", "description": "读取文档"}
        for update in ({"description": None}, {"description": []}, {"description": " "},
                       {"description": "字" * 1025}, {"name": "a" * 65},
                       {"name": "wrong-name"}, {"user-invocable": True},
                       {"metadata": {"version": 1}}, {"compatibility": "a" * 501}):
            with self.subTest(update=update):
                self.assertTrue(validate_metadata({**base, **update}, "feishu-cli-test"))
        self.assertEqual(validate_metadata({**base, "description": "字" * 1024}, "feishu-cli-test"), [])


class ExampleTests(unittest.TestCase):
    def setUp(self):
        self.catalog = {
            (): {"flags": {"--profile": True}, "aliases": []},
            ("calendar",): {"flags": {"--profile": True}, "aliases": []},
            ("calendar", "attendee"): {"flags": {}, "aliases": ["attendees"]},
            ("calendar", "attendee", "add"): {"flags": {"--profile": True, "--attendees": True, "--dry-run": False}, "aliases": []},
            ("doc",): {"flags": {}, "aliases": []},
            ("doc", "content-update"): {"flags": {"--markdown": True, "--mode": True, "--upload-images": False}, "aliases": []},
        }

    def test_wrong_calendar_flags_are_detected(self):
        status, bad, command = check_example("feishu-cli calendar attendee add --calendar-id x --event-id y", self.catalog)
        self.assertEqual((status, bad, command), ("checked", ["--calendar-id", "--event-id"], "calendar attendee add"))

    def test_alias_prefix_flag_quotes_comments_and_pipeline(self):
        command = 'feishu-cli --profile fixture calendar attendees add calendar_x event_x --attendees \'[{"id":"--not-a-flag"}]\' --dry-run | jq --raw-output . # --wrong'
        self.assertEqual(check_example(command, self.catalog)[1], [])
        self.assertEqual(check_example('feishu-cli doc content-update doc_x --markdown "--not-a-flag" --mode append', self.catalog)[1], [])

    def test_templates_skipped_and_fenced_continuations(self):
        self.assertEqual(check_example("feishu-cli calendar attendee ${ACTION} --some-flag", self.catalog)[0], "skipped")
        text = "outside feishu-cli bad\n```bash\nfeishu-cli doc content-update doc_x \\\n  --markdown text\n```\n```json\nfeishu-cli bogus --bad\n```\n"
        self.assertEqual(list(examples(text)), [(3, "feishu-cli doc content-update doc_x  --markdown text")])

    def test_unknown_prefix_flag_and_subcommands_are_rejected(self):
        result = check_example("feishu-cli --profil fixture calendar attendee add cal evt", self.catalog)
        self.assertEqual(result, ("checked", ["--profil"], "<root>"))
        for command in ("feishu-cli calendar attenddee list", "feishu-cli calendr attendee add"):
            with self.subTest(command=command):
                result = check_example(command, self.catalog)
                self.assertEqual(result[0], "invalid")
                self.assertIn("不存在的子命令", result[1][0])

    def test_runnable_group_accepts_positional_argument(self):
        help_text = "Usage:\n  feishu-cli schema [method] [flags]\n  feishu-cli schema [command]\nFlags:\n      --format string   输出\n"
        catalog = {**self.catalog, ("schema",): parse_help(help_text),
                   ("schema", "status"): {"flags": {}, "aliases": []}}
        self.assertEqual(check_example("feishu-cli schema im.messages.get --format json", catalog)[1], [])
        self.assertEqual(check_example("feishu-cli schema im.messages.get --wrong", catalog)[1], ["--wrong"])

    def test_command_substitution_includes_flags_and_ignores_literals(self):
        text = "\n".join([
            "```bash",
            'PRES_ID=$(feishu-cli calendar attendee add cal evt --calendar-id bad|jq -r .id)',
            'echo "$(feishu-cli doc content-update doc_x --wrong)"',
            "echo '$(feishu-cli bogus --not-a-command)'",
            '# SKIP=$(feishu-cli bogus --not-a-command)',
            'INCOMPLETE=$(feishu-cli calendar attendee add --wrong',
            "QUOTED=$(feishu-cli doc content-update --markdown 'unfinished",
            "```",
        ])
        extracted = list(examples(text))
        self.assertEqual([line for line, _ in extracted], [2, 3, 6, 7])
        self.assertEqual(check_example(extracted[0][1], self.catalog)[1], ["--calendar-id"])
        self.assertEqual(check_example(extracted[1][1], self.catalog)[1], ["--wrong"])
        self.assertEqual(check_example(extracted[2][1], self.catalog)[0], "skipped")
        self.assertEqual(check_example(extracted[3][1], self.catalog)[0], "skipped")

    def test_help_ignores_description_flag_mentions(self):
        help_text = "说明: --wrong 不是真的参数\n\nAliases:\n  attendee, attendees\n\nFlags:\n  -h, --help   help\n      --name string   名称\nGlobal Flags:\n      --debug   调试\n"
        self.assertEqual(parse_help(help_text), {"flags": {"--help": False, "-h": False, "--name": True, "--debug": False}, "aliases": ["attendee", "attendees"], "runnable": False})


class ExplicitGroupPrefixTests(unittest.TestCase):
    OWNERS = [
        ("s", "send", ("msg",)),
        ("s", "send", ("msg", "send")),
        ("s", "read", ("msg", "get")),
        ("s", "read", ("msg", "reaction")),
        ("s", "sheet", ("sheet",)),
    ]

    def test_split_group_requires_explicit_prefix_for_every_subcommand(self):
        commands = [("msg",), ("msg", "send"), ("msg", "get"), ("msg", "reaction"), ("msg", "reaction", "add"), ("msg", "urgent")]
        errors = check_explicit_group_prefixes(commands, self.OWNERS)
        self.assertEqual(len(errors), 1, errors)
        self.assertIn("msg urgent 只靠长度 1 的兜底前缀归属", errors[0])
        self.assertIn("长度 ≥2", errors[0])
        # 补上显式前缀后通过；组路径本身 ("msg",) 仍可只靠兜底前缀
        owners = self.OWNERS + [("s", "send", ("msg", "urgent"))]
        self.assertEqual(check_explicit_group_prefixes(commands, owners), [])

    def test_single_workflow_group_may_rely_on_fallback(self):
        commands = [("sheet",), ("sheet", "read"), ("sheet", "image"), ("sheet", "image", "add")]
        self.assertEqual(check_explicit_group_prefixes(commands, self.OWNERS), [])

    def test_nested_split_group_requires_deeper_prefix(self):
        owners = [
            ("s", "a", ("x",)),
            ("s", "a", ("x", "g")),
            ("s", "b", ("x", "g", "one")),
        ]
        commands = [("x", "g", "one"), ("x", "g", "two")]
        errors = check_explicit_group_prefixes(commands, owners)
        self.assertEqual(len(errors), 1, errors)
        self.assertIn("命令组 x g", errors[0])
        self.assertIn("x g two 只靠长度 2", errors[0])
        self.assertIn("长度 ≥3", errors[0])

    def test_resolve_owner_longest_prefix(self):
        self.assertEqual(resolve_owner(("msg", "get"), self.OWNERS), ({("s", "read")}, 2))
        self.assertEqual(resolve_owner(("msg", "urgent"), self.OWNERS), ({("s", "send")}, 1))
        self.assertEqual(resolve_owner(("nope",), self.OWNERS), (set(), 0))

    def test_real_manifest_catches_new_subcommand_in_split_groups(self):
        manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
        owners = [(o["skill"], o["workflow"], tuple(p)) for o in manifest["owners"] for p in o["prefixes"]]
        # 真实 manifest 下这些跨工作流组的现有子命令都有显式前缀
        existing = [
            ("msg", "send"), ("msg", "delete"), ("msg", "history"), ("msg", "resource-download"), ("msg", "flag", "create"),
            ("doc", "import"), ("doc", "import-file"), ("doc", "htmlbox", "create"), ("doc", "table", "insert-row"),
            ("drive", "upload"), ("drive", "search"), ("drive", "add-comment"), ("drive", "secure-label", "set"),
        ]
        self.assertEqual(check_explicit_group_prefixes(existing, owners), [])
        for group in ("msg", "doc", "drive"):
            with self.subTest(group=group):
                errors = check_explicit_group_prefixes(existing + [(group, "brand-new")], owners)
                self.assertEqual(len(errors), 1, errors)
                self.assertIn(f"{group} brand-new 只靠长度 1 的兜底前缀归属", errors[0])


class TriggerTests(unittest.TestCase):
    def test_default_sets_remain_eight_positive_eight_neighbor_negative(self):
        skills = [f"skill-{i}" for i in range(9)]
        rows = [{"expected_skill": skill, "query": f"{skill} query {i}"} for skill in skills for i in range(8)]
        for skill in skills:
            result = build_eval_set(skill, rows, skills)
            self.assertEqual(sum(row["should_trigger"] for row in result), 8)
            self.assertEqual(len(result), 16)
            self.assertEqual(len({row["query"] for row in result}), 16)


if __name__ == "__main__":
    unittest.main()
