package docxparse

import "testing"

// 以下用例补充官方测试未直接覆盖的分支（反向验证时发现）。

func TestNormalizeCompatibleXMLRewritesLegacyBlockID(t *testing.T) {
	for source, want := range map[string]string{
		`<block_id="8,9"/><p>x</p>`:                     `<block_id>8,9</block_id><p>x</p>`,
		`<block_id="8"></block_id><p>x</p>`:             `<block_id>8</block_id><p>x</p>`,
		`<block_id="8"><p>x</p>`:                        `<block_id>8</block_id><p>x</p>`,
		`<p>keep <b>bold</b></p><block_id>1</block_id>`: `<p>keep <b>bold</b></p><block_id>1</block_id>`,
	} {
		if got := normalizeCompatibleXMLInput(source); got != want {
			t.Errorf("normalizeCompatibleXMLInput(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestDualLayoutTagCountsOnlyAtTopLevel(t *testing.T) {
	result, err := Parse(`<p>a <code>inline</code></p><code>top</code><p><latex>x</latex></p>`, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := blockCountForTest(result.Profile.Blocks, "code"); got != 1 {
		t.Fatalf("code blocks = %d, want 1 (only top-level code is a block); profile=%+v", got, result.Profile)
	}
	if got := blockCountForTest(result.Profile.Blocks, "latex"); got != 0 {
		t.Fatalf("inline latex must not be counted as a block; profile=%+v", result.Profile)
	}
	if result.Profile.BlockCount != 3 {
		t.Fatalf("block_count = %d, want 3 (two p + top-level code)", result.Profile.BlockCount)
	}
}
