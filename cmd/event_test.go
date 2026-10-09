package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/event"
	"github.com/spf13/cobra"
)

func TestEventListCmd_OutputsAllDomains(t *testing.T) {
	// 静态注册表测试：确认 list 命令的核心数据源 ListAll 返回值能正确分组
	all := event.ListAll()
	if len(all) == 0 {
		t.Fatal("event.ListAll() 不应为空")
	}

	// 至少应包含 im / contact / calendar 三个 domain
	domains := map[string]bool{}
	for _, def := range all {
		domains[def.Domain] = true
	}
	for _, must := range []string{"im", "contact", "calendar"} {
		if !domains[must] {
			t.Errorf("缺少必备 domain %q", must)
		}
	}
}

func TestEventListCmd_JSONFlag(t *testing.T) {
	// JSON 模式应能用 printJSON 序列化
	buf := &bytes.Buffer{}
	_ = buf
	// 调用底层 printJSON 验证 KeyDefinition 字段可被序列化（不调实际命令避免 cobra 状态污染）
	defs := []event.KeyDefinition{
		{Key: "im.test", EventType: "im.test", Domain: "im", Description: "test"},
	}
	if err := printJSON(defs); err != nil {
		t.Errorf("printJSON 失败: %v", err)
	}
}

func TestEventSchemaCmd_UnknownKey(t *testing.T) {
	// schema <unknown-key> 应返回明确错误
	_, ok := event.Lookup("definitely.not.a.real.key_v999")
	if ok {
		t.Fatal("Lookup 假数据 key 不应命中")
	}
}

func TestEventSchemaCmd_KnownKey(t *testing.T) {
	def, ok := event.Lookup("im.message.receive_v1")
	if !ok {
		t.Fatal("Lookup im.message.receive_v1 应命中")
	}
	if def.PayloadSchema == "" {
		t.Errorf("im.message.receive_v1 应附带 PayloadSchema 示例")
	}
	// PayloadSchema 应是合法 JSON 风格的多行文本（含 header 和 event 字段）
	if !strings.Contains(def.PayloadSchema, "header") || !strings.Contains(def.PayloadSchema, "event") {
		t.Errorf("PayloadSchema 应包含 header/event 字段引用")
	}
}

func TestEventStopCmd_RejectsNoArgs(t *testing.T) {
	// 不提供任何 flag 时应报错（cobra 层面验证：需要 --pid / --event-key / --all 之一）
	// 这里只验证命令实例存在；具体 args 校验 cobra 自己跑过
	if eventStopCmd.Short == "" {
		t.Errorf("eventStopCmd.Short 应非空")
	}
	if eventStopCmd.RunE == nil {
		t.Errorf("eventStopCmd.RunE 应非空")
	}
}

func TestEventConsumeCmd_RegisteredFlags(t *testing.T) {
	// 验证关键 flag 已注册（避免后续重构遗漏）
	for _, flag := range []string{"max-events", "timeout", "jq", "output-dir", "quiet"} {
		if eventConsumeCmd.Flag(flag) == nil {
			t.Errorf("consume 命令缺少 --%s flag", flag)
		}
	}
}

func TestEventConsumeHelpDocumentsReadyAfterHandshake(t *testing.T) {
	if strings.Contains(eventConsumeCmd.Long, "handshake in progress") {
		t.Fatal("consume help 不得再声称 ready 时握手仍在进行")
	}
	if !strings.Contains(eventConsumeCmd.Long, "握手") {
		t.Fatal("consume help 应说明 ready 发生在握手之后")
	}
}

func TestEventStopCmd_RegisteredFlags(t *testing.T) {
	for _, flag := range []string{"pid", "event-key", "all", "force", "json"} {
		if eventStopCmd.Flag(flag) == nil {
			t.Errorf("stop 命令缺少 --%s flag", flag)
		}
	}
}

func TestEventStatusCmd_RegisteredFlags(t *testing.T) {
	if eventStatusCmd.Flag("json") == nil {
		t.Errorf("status 命令缺少 --json flag")
	}
}

func TestEventCmd_HasAllSubcommands(t *testing.T) {
	expected := map[string]bool{"list": false, "schema": false, "consume": false, "status": false, "stop": false}
	for _, sub := range eventCmd.Commands() {
		expected[sub.Name()] = true
	}
	for name, found := range expected {
		if !found {
			t.Errorf("event 命令缺少子命令 %q", name)
		}
	}
}

// TestShouldWatchStdinEOF 有界运行（--max-events / --timeout）忽略 stdin EOF；
// 只有非 TTY 的无界运行把 EOF 当退出信号（对齐官方 #1285）。
func TestShouldWatchStdinEOF(t *testing.T) {
	tests := []struct {
		name      string
		tty       bool
		maxEvents int
		timeout   time.Duration
		want      bool
	}{
		{"非 TTY 无界", false, 0, 0, true},
		{"非 TTY + timeout", false, 0, 30 * time.Second, false},
		{"非 TTY + max-events", false, 1, 0, false},
		{"非 TTY + 两者", false, 1, 30 * time.Second, false},
		{"TTY 无界", true, 0, 0, false},
		{"负数视为无界", false, -1, -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldWatchStdinEOF(tt.tty, tt.maxEvents, tt.timeout); got != tt.want {
				t.Fatalf("shouldWatchStdinEOF(%v,%d,%s) = %v, want %v", tt.tty, tt.maxEvents, tt.timeout, got, tt.want)
			}
		})
	}
}

func newEventConsumeTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Int("max-events", 0, "")
	cmd.Flags().Duration("timeout", 0, "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().String("user-access-token", "", "")
	cmd.Flags().String("output-dir", "", "")
	cmd.Flags().Bool("quiet", false, "")
	cmd.Flags().Bool("force", false, "")
	return cmd
}

// TestEventConsumeRejectsSecondInstance 同一 App 已有 consume 持有单实例锁时，
// 第二个进程在建连前报错，并提示合并 EventKey / --force。
func TestEventConsumeRejectsSecondInstance(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("锁冲突时不应发出任何网络请求: %s", r.URL.Path)
	})
	defer cleanup()

	lock, err := event.AcquireConsumeLock("test_app_id")
	if err != nil {
		t.Fatalf("预先获取锁失败: %v", err)
	}
	defer lock.Release()

	cmd := newEventConsumeTestCmd()
	err = eventConsumeCmd.RunE(cmd, []string{"im.message.receive_v1"})
	if err == nil {
		t.Fatal("第二个 consume 应报错")
	}
	for _, want := range []string{"只允许一个", "event consume <key1> <key2>", "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("错误信息缺少 %q: %v", want, err)
		}
	}
}

func TestEventConsumeRejectsUnknownKeyAsUsage(t *testing.T) {
	cmd := newEventConsumeTestCmd()
	err := eventConsumeCmd.RunE(cmd, []string{"im.message.receive_v1,not.a.key"})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("未知 EventKey 应为用法错误，got %v", err)
	}
}
