package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

func TestSanitizeWikiTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal", "Hello World", "Hello_World"},
		{"chinese", "学习计划", "学习计划"},
		{"slash", "a/b/c", "a_b_c"},
		{"backslash", `a\b\c`, "a_b_c"},
		{"colon", "a:b", "a_b"},
		{"asterisk", "a*b", "a_b"},
		{"question", "a?b", "a_b"},
		{"quote", `a"b`, "a_b"},
		{"angle brackets", "a<b>c", "a_b_c"},
		{"pipe", "a|b", "a_b"},
		{"all special", `a/\:*?"<>| b`, "a__________b"},
		{"empty", "", "untitled"},
		{"only spaces", "   ", "untitled"},
		{"only dots", "...", "untitled"},
		{"trailing dots and spaces", "  hello.  ", "hello"},
		{"emoji preserved", "Go 学习指引 ⭐", "Go_学习指引_⭐"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeWikiTitle(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeWikiTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeWikiTitleLengthLimit(t *testing.T) {
	// safeOutputPath 在 utils.go 里的硬上限是 200 字符
	long := strings.Repeat("a", 250)
	got := sanitizeWikiTitle(long)
	if len(got) > 200 {
		t.Errorf("sanitizeWikiTitle did not truncate to <=200, got len=%d", len(got))
	}
}

func TestDedupeSiblingName(t *testing.T) {
	used := map[string]bool{}

	// 第一次出现：原名直接用
	first := dedupeSiblingName("hello", "abcdef123456", used)
	if first != "hello" {
		t.Fatalf("first occurrence should keep original name, got %q", first)
	}
	used[first] = true

	// 第二次同名：加 token 前 6 位
	second := dedupeSiblingName("hello", "xyz789abc000", used)
	wantSecond := "hello_xyz789"
	if second != wantSecond {
		t.Fatalf("second occurrence should append token prefix, got %q want %q", second, wantSecond)
	}
	used[second] = true

	// 第三次同名同 token 前缀（构造极端碰撞）：加序号
	third := dedupeSiblingName("hello", "xyz789other", used)
	if third == "hello" || third == "hello_xyz789" {
		t.Fatalf("third occurrence collided, got %q", third)
	}
	if !strings.HasPrefix(third, "hello_xyz789") {
		t.Fatalf("third should still start with hello_xyz789, got %q", third)
	}
}

func TestDedupeSiblingNameShortToken(t *testing.T) {
	used := map[string]bool{"x": true}
	got := dedupeSiblingName("x", "abc", used)
	wantSuffix := "_abc"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("short token should be used as-is, got %q want suffix %q", got, wantSuffix)
	}
}

func TestIsExportableWikiType(t *testing.T) {
	tests := []struct {
		name    string
		objType string
		allowed []string
		want    bool
	}{
		{"docx default", "docx", []string{"docx", "sheet"}, true},
		{"sheet default", "sheet", []string{"docx", "sheet"}, true},
		{"bitable rejected", "bitable", []string{"docx", "sheet"}, false},
		{"file rejected", "file", []string{"docx", "sheet"}, false},
		{"mindnote rejected", "mindnote", []string{"docx", "sheet"}, false},
		{"slides rejected", "slides", []string{"docx", "sheet"}, false},
		{"doc rejected", "doc", []string{"docx", "sheet"}, false},
		{"narrow to docx only", "sheet", []string{"docx"}, false},
		{"narrow to docx only - docx ok", "docx", []string{"docx"}, true},
		{"empty allowed list = all supported", "docx", nil, true},
		{"empty allowed list = all supported sheet", "sheet", []string{}, true},
		{"empty allowed still rejects unsupported", "bitable", nil, false},
		{"case insensitive", "DOCX", []string{"docx"}, false}, // ObjType is always lowercase
		{"whitespace in allowed", "docx", []string{"  docx  "}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExportableWikiType(tt.objType, tt.allowed)
			if got != tt.want {
				t.Errorf("isExportableWikiType(%q, %v) = %v, want %v", tt.objType, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestFileExistsAndNonEmpty(t *testing.T) {
	dir := t.TempDir()

	t.Run("nonexistent", func(t *testing.T) {
		if fileExistsAndNonEmpty(filepath.Join(dir, "nope.md")) {
			t.Error("nonexistent file should return false")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		p := filepath.Join(dir, "empty.md")
		if err := os.WriteFile(p, []byte{}, 0600); err != nil {
			t.Fatal(err)
		}
		if fileExistsAndNonEmpty(p) {
			t.Error("empty file should return false")
		}
	})

	t.Run("nonempty file", func(t *testing.T) {
		p := filepath.Join(dir, "ok.md")
		if err := os.WriteFile(p, []byte("hello"), 0600); err != nil {
			t.Fatal(err)
		}
		if !fileExistsAndNonEmpty(p) {
			t.Error("non-empty file should return true")
		}
	})

	t.Run("directory returns false", func(t *testing.T) {
		if fileExistsAndNonEmpty(dir) {
			t.Error("directory should return false (not a file)")
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"shorter than limit", "hello", 10, "hello"},
		{"exactly at limit", "hello", 5, "hello"},
		{"longer than limit", "hello world", 5, "hello…"},
		{"empty", "", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

func TestWikiTreeNodeAssetsDirUsesPerDocumentDirectory(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("download-images", true, "")
	cmd.Flags().String("assets-dir", "backup/assets", "")

	job := treeJob{
		Node:       &client.WikiNode{ObjType: "docx"},
		OutputPath: filepath.Join("backup", "Team", "Plan.md"),
	}

	got := wikiTreeNodeAssetsDir(cmd, "backup", job)
	want := filepath.Join("backup", "assets", "Team", "Plan")
	if got != want {
		t.Fatalf("wikiTreeNodeAssetsDir() = %q, want %q", got, want)
	}
}

func TestWikiTreeNodeAssetsDirSkipsWhenDisabled(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("download-images", false, "")
	cmd.Flags().String("assets-dir", "assets", "")

	got := wikiTreeNodeAssetsDir(cmd, "backup", treeJob{
		Node:       &client.WikiNode{ObjType: "docx"},
		OutputPath: filepath.Join("backup", "Plan.md"),
	})
	if got != "" {
		t.Fatalf("wikiTreeNodeAssetsDir() = %q, want empty", got)
	}
}

func TestMakeImagePathsDocumentRelative(t *testing.T) {
	tests := []struct {
		name       string
		markdown   string
		assetsDir  string
		outputPath string
		want       string
	}{
		{
			name:       "simple relative",
			markdown:   "![image](backup/assets/Team/Plan/image_1.png)",
			assetsDir:  filepath.Join("backup", "assets", "Team", "Plan"),
			outputPath: filepath.Join("backup", "Team", "Plan.md"),
			want:       "![image](../assets/Team/Plan/image_1.png)",
		},
		{
			name:       "deep nested",
			markdown:   "![image](rbk3.4_api/assets/TCP_IP_API/overview/coord/image_1.png)",
			assetsDir:  filepath.Join("rbk3.4_api", "assets", "TCP_IP_API", "overview", "coord"),
			outputPath: filepath.Join("rbk3.4_api", "TCP_IP_API", "overview", "coord.md"),
			want:       "![image](../../assets/TCP_IP_API/overview/coord/image_1.png)",
		},
		{
			name:       "empty assets dir",
			markdown:   "![image](image_1.png)",
			assetsDir:  "",
			outputPath: "some/file.md",
			want:       "![image](image_1.png)",
		},
		{
			name:       "no image refs",
			markdown:   "# Hello World\n\nSome text.\n",
			assetsDir:  "assets",
			outputPath: "file.md",
			want:       "# Hello World\n\nSome text.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeImagePathsDocumentRelative(tt.markdown, tt.assetsDir, tt.outputPath)
			if got != tt.want {
				t.Errorf("makeImagePathsDocumentRelative() = %q, want %q", got, tt.want)
			}
		})
	}
}
