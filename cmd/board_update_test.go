package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBoardUpdateFlags(t *testing.T) {
	if boardUpdateCmd.Flag("overwrite") == nil {
		t.Error("board update 缺少 --overwrite flag")
	}
	if boardUpdateCmd.Flag("snapshot") == nil {
		t.Error("board update 缺少 --snapshot flag")
	}
	if boardUpdateCmd.Flag("dry-run") == nil {
		t.Error("board update 缺少 --dry-run flag")
	}
	if boardUpdateCmd.Flag("stdin") == nil {
		t.Error("board update 缺少 --stdin flag")
	}
	if !strings.Contains(boardUpdateCmd.Long, "overwrite: true") {
		t.Errorf("board update Long 说明应包含 overwrite: true 说明: %s", boardUpdateCmd.Long)
	}
}

func TestBoardCreateNotesFlags(t *testing.T) {
	if createBoardNotesCmd.Flag("client-token") == nil {
		t.Error("board create-notes 缺少 --client-token flag")
	}
	if createBoardNotesCmd.Flag("overwrite") == nil {
		t.Error("board create-notes 缺少 --overwrite flag")
	}
	// 验证示例中不包含短 token abc123
	if strings.Contains(createBoardNotesCmd.Long, "--client-token abc123") {
		t.Errorf("board create-notes 示例不应包含短 token abc123")
	}
}

func TestBoardImportDiagramFlags(t *testing.T) {
	if importDiagramCmd.Flag("parse-mode") == nil {
		t.Error("board import 缺少 --parse-mode flag")
	}
	if importDiagramCmd.Flag("overwrite") == nil {
		t.Error("board import 缺少 --overwrite flag")
	}
}

// TestParseBoardNodeIDs 覆盖 data.nodes 的几种形态：缺失（空画板）、null、数组、map。
func TestParseBoardNodeIDs(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"缺失 nodes（空画板）", "", []string{}},
		{"null", "null", []string{}},
		{"空数组", "[]", []string{}},
		{"数组", `[{"id":"o1:1"},{"id":"o1:2"},{"type":"x"}]`, []string{"o1:1", "o1:2"}},
		{"map", `{"o1:1":{"type":"text_shape"}}`, []string{"o1:1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBoardNodeIDs(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("parseBoardNodeIDs(%q) error = %v", tc.raw, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("parseBoardNodeIDs(%q) = %v, want %v", tc.raw, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("parseBoardNodeIDs(%q)[%d] = %q, want %q", tc.raw, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestBoardDeleteAll_EmptyBoard 回归：空画板 GET /nodes 返回 {"code":0,"data":{}}（没有 nodes 字段），
// board delete --all 应提示"没有节点"并成功退出，且不发出 batch_delete。
func TestBoardDeleteAll_EmptyBoard(t *testing.T) {
	var deleteCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/whiteboards/wb_empty/nodes"):
			_, _ = io.WriteString(w, `{"code":0,"data":{},"msg":""}`)
		case strings.HasSuffix(r.URL.Path, "/nodes/batch_delete"):
			deleteCalls++
			_, _ = io.WriteString(w, `{"code":0,"data":{},"msg":""}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))

	c := &cobra.Command{Use: "delete"}
	c.Flags().String("node-ids", "", "")
	c.Flags().Bool("all", false, "")
	c.Flags().StringP("output", "o", "", "")
	c.Flags().String("user-access-token", "", "")
	if err := c.Flags().Set("all", "true"); err != nil {
		t.Fatal(err)
	}

	out, err := captureAppsStdout(t, func() error { return boardDeleteCmd.RunE(c, []string{"wb_empty"}) })
	if err != nil {
		t.Fatalf("空画板 delete --all 不应报错: %v", err)
	}
	if !strings.Contains(out, "没有节点") {
		t.Fatalf("应提示画板没有节点，实际输出: %q", out)
	}
	if deleteCalls != 0 {
		t.Fatalf("空画板不应调用 batch_delete，实际 %d 次", deleteCalls)
	}
}
