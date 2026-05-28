package cmd

import (
	"strings"
	"testing"
)

// TestMarkdownDiffCmdRegistered 验证 diff 子命令注册 + file-token 必填
func TestMarkdownDiffCmdRegistered(t *testing.T) {
	if markdownDiffCmd.Use != "diff" {
		t.Fatalf("Use = %q, want diff", markdownDiffCmd.Use)
	}
	found := false
	for _, sub := range markdownCmd.Commands() {
		if sub == markdownDiffCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("markdownDiffCmd should be child of markdownCmd")
	}
	f := markdownDiffCmd.Flags().Lookup("file-token")
	if f == nil {
		t.Fatal("--file-token missing")
	}
	ann := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]
	if len(ann) == 0 || ann[0] != "true" {
		t.Errorf("--file-token should be required, ann=%v", ann)
	}
	for _, n := range []string{"file", "from-version", "to-version", "obj-type", "context-lines", "dry-run", "output"} {
		if markdownDiffCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on diff", n)
		}
	}
	if cl := markdownDiffCmd.Flags().Lookup("context-lines"); cl == nil || cl.DefValue != "3" {
		t.Errorf("--context-lines default = %v, want 3", cl)
	}
}

// TestResolveMarkdownDiffMode 验证三种模式判定 + 互斥/缺省报错
func TestResolveMarkdownDiffMode(t *testing.T) {
	cases := []struct {
		name                      string
		localFile, fromVer, toVer string
		wantMode                  string
		wantErr                   bool
	}{
		{"local vs remote", "a.md", "", "", "local_vs_remote", false},
		{"remote version vs latest", "", "3", "", "remote_vs_remote", false},
		{"remote version vs version", "", "2", "5", "remote_vs_remote", false},
		{"to without from", "", "", "5", "", true},
		{"local + version mutually exclusive", "a.md", "2", "", "", true},
		{"nothing", "", "", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveMarkdownDiffMode(tc.localFile, tc.fromVer, tc.toVer)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got mode=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantMode {
				t.Fatalf("mode = %q, want %q", got, tc.wantMode)
			}
		})
	}
}

// TestUnifiedDiffIdentical 相同内容应无 hunk
func TestUnifiedDiffIdentical(t *testing.T) {
	a := "line1\nline2\nline3\n"
	if hunks := unifiedDiff(a, a, 3); len(hunks) != 0 {
		t.Fatalf("identical content should yield 0 hunks, got %d", len(hunks))
	}
}

// TestUnifiedDiffAddRemoveModify 验证新增/删除/修改行的 op 与行号
func TestUnifiedDiffAddRemoveModify(t *testing.T) {
	a := "alpha\nbeta\ngamma\n"
	b := "alpha\nBETA\ngamma\ndelta\n"
	hunks := unifiedDiff(a, b, 1)
	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}
	added, removed := countDiffLines(hunks)
	// beta -> BETA = 1 删 1 增；delta = 1 增 => added=2 removed=1
	if added != 2 {
		t.Errorf("added = %d, want 2", added)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	// 验证修改行确实表现为 -beta / +BETA
	var sawRemoveBeta, sawAddBETA, sawAddDelta bool
	for _, h := range hunks {
		for _, l := range h.Lines {
			switch {
			case l.Op == "-" && l.Text == "beta":
				sawRemoveBeta = true
			case l.Op == "+" && l.Text == "BETA":
				sawAddBETA = true
			case l.Op == "+" && l.Text == "delta":
				sawAddDelta = true
			}
		}
	}
	if !sawRemoveBeta || !sawAddBETA || !sawAddDelta {
		t.Errorf("missing expected ops: -beta=%v +BETA=%v +delta=%v", sawRemoveBeta, sawAddBETA, sawAddDelta)
	}

	// 首个 hunk 行号从 1 开始
	if hunks[0].OldStart != 1 || hunks[0].NewStart != 1 {
		t.Errorf("first hunk start = old %d new %d, want 1/1", hunks[0].OldStart, hunks[0].NewStart)
	}
}

// TestUnifiedDiffContextLines 验证 context-lines 控制上下文行数
func TestUnifiedDiffContextLines(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n"
	b := "1\n2\n3\nX\n5\n6\n7\n" // 第 4 行 4 -> X

	// context=0：hunk 只含变更行（-4 +X），无未变更上下文
	h0 := unifiedDiff(a, b, 0)
	if len(h0) != 1 {
		t.Fatalf("context=0 expected 1 hunk, got %d", len(h0))
	}
	for _, l := range h0[0].Lines {
		if l.Op == " " {
			t.Errorf("context=0 should have no unchanged lines, saw %q", l.Text)
		}
	}

	// context=2：变更行前后各 2 行上下文
	h2 := unifiedDiff(a, b, 2)
	if len(h2) != 1 {
		t.Fatalf("context=2 expected 1 hunk, got %d", len(h2))
	}
	var ctx int
	for _, l := range h2[0].Lines {
		if l.Op == " " {
			ctx++
		}
	}
	if ctx != 4 {
		t.Errorf("context=2 unchanged lines = %d, want 4", ctx)
	}
}

// TestRenderUnifiedDiff 验证 @@ 头格式与行前缀
func TestRenderUnifiedDiff(t *testing.T) {
	a := "a\nb\n"
	b := "a\nc\n"
	out := renderUnifiedDiff(unifiedDiff(a, b, 0))
	if !strings.Contains(out, "@@ -") || !strings.Contains(out, "+") {
		t.Fatalf("missing hunk header in:\n%s", out)
	}
	if !strings.Contains(out, "-b\n") || !strings.Contains(out, "+c\n") {
		t.Fatalf("missing change lines in:\n%s", out)
	}
}

// TestCheckDiffSize 验证 diff 行数上限拦截
func TestCheckDiffSize(t *testing.T) {
	small := strings.Repeat("x\n", 10)
	if err := checkDiffSize(small, small); err != nil {
		t.Fatalf("small content should pass, got %v", err)
	}

	// 任一侧超过 maxDiffLines 应报错
	big := strings.Repeat("x\n", maxDiffLines+1)
	if err := checkDiffSize(big, small); err == nil {
		t.Fatal("from over limit should error")
	}
	if err := checkDiffSize(small, big); err == nil {
		t.Fatal("to over limit should error")
	}

	// 单侧正好等于行数上限、且与另一侧乘积不超 maxDiffCells → 放行
	atLimit := strings.Repeat("x\n", maxDiffLines)
	if err := checkDiffSize(atLimit, small); err != nil {
		t.Fatalf("single side at line limit (small other side) should pass, got %v", err)
	}

	// CRITICAL 保护：两侧各自不超行数上限，但行数乘积超过 maxDiffCells 也应报错（防 LCS 矩阵 OOM）
	midLines := strings.Repeat("x\n", maxDiffCells/maxDiffLines+1) // < maxDiffLines 行，但 × atLimit 乘积超限
	if err := checkDiffSize(atLimit, midLines); err == nil {
		t.Fatal("product over maxDiffCells should error even when each side within line limit")
	}
}

// TestSplitDiffLines 验证 CRLF 规整与行尾换行处理
func TestSplitDiffLines(t *testing.T) {
	got := splitDiffLines("a\r\nb\n")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitDiffLines = %#v, want [a b]", got)
	}
	if got := splitDiffLines(""); got != nil {
		t.Fatalf("empty should be nil, got %#v", got)
	}
}
