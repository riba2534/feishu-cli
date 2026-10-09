package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newThreadMessagesTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("sort", "ByCreateTimeAsc", "")
	cmd.Flags().Int("page-size", 50, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().String("start-time", "", "")
	cmd.Flags().String("end-time", "", "")
	cmd.Flags().String("user-access-token", "", "")
	return cmd
}

// TestThreadMessagesFiltersLocallyBySeconds 验证 thread-messages 的时间参数：
//  1. 不把 start_time/end_time 发给服务端（thread 容器服务端忽略时间范围）；
//  2. 按秒级参数在本地按 create_time（毫秒）过滤，结束时间含整秒边界；
//  3. has_more 时 stderr 提示继续翻页。
func TestThreadMessagesFiltersLocallyBySeconds(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var serverQuery string
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/im/v1/messages" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		serverQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"has_more":true,"page_token":"next","items":[
			{"message_id":"om_before","create_time":"1700000099999"},
			{"message_id":"om_start","create_time":"1700000100000"},
			{"message_id":"om_mid","create_time":"1700000150000"},
			{"message_id":"om_end_edge","create_time":"1700000200999"},
			{"message_id":"om_after","create_time":"1700000201000"}
		]}}`)
	}))
	defer cleanup()

	cmd := newThreadMessagesTestCmd()
	mustSetFlag(t, cmd, "start-time", "1700000100")
	mustSetFlag(t, cmd, "end-time", "1700000200")
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	out := captureStdout(t, func() {
		if err := msgThreadMessagesCmd.RunE(cmd, []string{"omt_thread"}); err != nil {
			t.Fatalf("thread-messages 返回错误: %v", err)
		}
	})

	if strings.Contains(serverQuery, "start_time") || strings.Contains(serverQuery, "end_time") {
		t.Fatalf("不应把时间范围发给服务端（thread 容器会忽略）: %s", serverQuery)
	}
	if !strings.Contains(serverQuery, "container_id_type=thread") {
		t.Fatalf("应按 thread 容器请求: %s", serverQuery)
	}

	var got struct {
		Items []struct {
			MessageID string `json:"message_id"`
		}
		HasMore bool
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("解析输出失败: %v\n%s", err, out)
	}
	var ids []string
	for _, it := range got.Items {
		ids = append(ids, it.MessageID)
	}
	if want := "om_start,om_mid,om_end_edge"; strings.Join(ids, ",") != want {
		t.Fatalf("本地过滤结果 = %v, want %s", ids, want)
	}
	if !got.HasMore {
		t.Fatal("has_more 应保持服务端原值")
	}
	if !strings.Contains(stderr.String(), "本地过滤") || !strings.Contains(stderr.String(), "--page-token") {
		t.Fatalf("stderr 应提示本地过滤与继续翻页，got %q", stderr.String())
	}
}

func TestParseLocalTimeWindow(t *testing.T) {
	tests := []struct {
		name      string
		start     string
		end       string
		wantStart int64
		wantEnd   int64
		wantErr   bool
	}{
		{"秒级", "1700000100", "1700000200", 1700000100000, 1700000200999, false},
		{"毫秒级兼容旧文档", "1700000100000", "1700000200500", 1700000100000, 1700000200500, false},
		{"只给起点", "1700000100", "", 1700000100000, 0, false},
		{"起点晚于终点", "1700000200", "1700000100", 0, 0, true},
		{"非法输入", "abc", "", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := parseLocalTimeWindow(tt.start, tt.end)
			if tt.wantErr {
				if err == nil {
					t.Fatal("期望报错")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if w.startMs != tt.wantStart || w.endMs != tt.wantEnd {
				t.Fatalf("window = %+v, want start=%d end=%d", w, tt.wantStart, tt.wantEnd)
			}
		})
	}
}
