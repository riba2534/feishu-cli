package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
)

// setupMailAttendanceCmdTestConfig 初始化 Mail/Attendance/Sheets 契约测试配置，将 base_url 指向 mock 服务器。
func setupMailAttendanceCmdTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")
	os.Unsetenv("FEISHU_USER_ACCESS_TOKEN")
	os.Unsetenv("FEISHU_PROFILE")
	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := fmt.Sprintf("app_id: \"cli_test_app\"\napp_secret: \"test_secret\"\nbase_url: \"%s\"\n", baseURL)
	if err := os.WriteFile(configFile, []byte(content), 0o600); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}
}

// TestSheetProtectCmd_AvailableAndWired 验证 sheet protect/unprotect 命令可用且参数完整。
// 回归防护：这两个命令曾被标记 Hidden 并直接返回 "unsupported"，
// 但实测 sheets v2 protected_dimension / protected_range_batch_del 端点在线可用
// （分别返回 code=0 与 delProtectIds），禁用等于删掉一个正常工作的能力。
func TestSheetProtectCmd_AvailableAndWired(t *testing.T) {
	if sheetProtectCmd.Hidden {
		t.Error("sheet protect 端点可用，命令不应隐藏")
	}
	if sheetUnprotectCmd.Hidden {
		t.Error("sheet unprotect 端点可用，命令不应隐藏")
	}
	if strings.Contains(sheetProtectCmd.Short, "unsupported") || strings.Contains(sheetProtectCmd.Short, "废弃") {
		t.Errorf("sheet protect Short 不应标记废弃: %q", sheetProtectCmd.Short)
	}

	for _, name := range []string{"dimension", "start", "end", "lock-info"} {
		if sheetProtectCmd.Flags().Lookup(name) == nil {
			t.Errorf("sheet protect 缺少 --%s flag", name)
		}
	}

	// protect 需要 <spreadsheet_token> <sheet_id> 两个位置参数
	if err := sheetProtectCmd.Args(sheetProtectCmd, []string{"only_one"}); err == nil {
		t.Error("sheet protect 应要求两个位置参数")
	}
	if err := sheetProtectCmd.Args(sheetProtectCmd, []string{"sht_token", "sheet_1"}); err != nil {
		t.Errorf("sheet protect 两个位置参数应合法: %v", err)
	}

	// unprotect 需要 token + 至少一个 protect_id
	if err := sheetUnprotectCmd.Args(sheetUnprotectCmd, []string{"sht_token"}); err == nil {
		t.Error("sheet unprotect 应要求至少一个 protect_id")
	}
	if err := sheetUnprotectCmd.Args(sheetUnprotectCmd, []string{"sht_token", "p1", "p2"}); err != nil {
		t.Errorf("sheet unprotect 多个 protect_id 应合法: %v", err)
	}
}

// TestAttendanceQuery_FlagsAndEnums 验证考勤 employee_type 仅支持 employee_id 和 employee_no
func TestAttendanceQuery_FlagsAndEnums(t *testing.T) {
	taskFlag := attendanceUserTaskQueryCmd.Flags().Lookup("employee-type")
	if taskFlag == nil || taskFlag.DefValue != "employee_id" {
		t.Errorf("attendance user-task query employee-type default = %v, want employee_id", taskFlag)
	}
	statsFlag := attendanceUserStatsQueryCmd.Flags().Lookup("employee-type")
	if statsFlag == nil || statsFlag.DefValue != "employee_id" {
		t.Errorf("attendance user-stats query employee-type default = %v, want employee_id", statsFlag)
	}

	// 验证 attendance user-task query 支持 --as 和 --user-access-token
	if attendanceUserTaskQueryCmd.Flags().Lookup("as") == nil {
		t.Error("attendance user-task query should have --as flag")
	}
	if attendanceUserTaskQueryCmd.Flags().Lookup("user-access-token") == nil {
		t.Error("attendance user-task query should have --user-access-token flag")
	}

	// 验证 attendance user-stats query 保持 legacy 路径，不暴露 --user-access-token
	if attendanceUserStatsQueryCmd.Flags().Lookup("user-access-token") != nil {
		t.Error("attendance user-stats query should NOT have --user-access-token flag in legacy path")
	}
}

// TestMailRead_AsBotRejectsMailboxMeBeforeNetwork 验证 Mail 读操作在 --as bot 时在发起网络请求前拒绝 mailbox=me
func TestMailRead_AsBotRejectsMailboxMeBeforeNetwork(t *testing.T) {
	networkCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		networkCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	// 1. mail triage: --as bot, default mailbox=me
	_ = mailTriageCmd.Flags().Set("as", "bot")
	_ = mailTriageCmd.Flags().Set("mailbox", "me")
	err := mailTriageCmd.RunE(mailTriageCmd, []string{})
	if err == nil {
		t.Fatal("mail triage --as bot with mailbox=me should return error")
	}
	if !strings.Contains(err.Error(), "不支持 mailbox=\"me\"") {
		t.Errorf("error = %v, want mentioning mailbox=me rejection", err)
	}
	if networkCalled {
		t.Error("before network: 不应发起任何网络请求")
	}

	// 2. mail messages: --as bot, default mailbox=me
	networkCalled = false
	_ = mailMessagesCmd.Flags().Set("as", "bot")
	_ = mailMessagesCmd.Flags().Set("mailbox", "me")
	_ = mailMessagesCmd.Flags().Set("message-ids", "msg_1")
	err = mailMessagesCmd.RunE(mailMessagesCmd, []string{})
	if err == nil {
		t.Fatal("mail messages --as bot with mailbox=me should return error")
	}
	if !strings.Contains(err.Error(), "不支持 mailbox=\"me\"") {
		t.Errorf("error = %v", err)
	}
	if networkCalled {
		t.Error("before network: 不应发起任何网络请求")
	}

	// 3. mail thread: --as bot, default mailbox=me
	networkCalled = false
	_ = mailThreadCmd.Flags().Set("as", "bot")
	_ = mailThreadCmd.Flags().Set("mailbox", "me")
	_ = mailThreadCmd.Flags().Set("thread-id", "th_1")
	err = mailThreadCmd.RunE(mailThreadCmd, []string{})
	if err == nil {
		t.Fatal("mail thread --as bot with mailbox=me should return error")
	}
	if !strings.Contains(err.Error(), "不支持 mailbox=\"me\"") {
		t.Errorf("error = %v", err)
	}
	if networkCalled {
		t.Error("before network: 不应发起任何网络请求")
	}

	// 4. mail message: --as bot, default mailbox=me
	networkCalled = false
	_ = mailMessageCmd.Flags().Set("as", "bot")
	_ = mailMessageCmd.Flags().Set("mailbox", "me")
	_ = mailMessageCmd.Flags().Set("message-id", "m1")
	err = mailMessageCmd.RunE(mailMessageCmd, []string{})
	if err == nil {
		t.Fatal("mail message --as bot with mailbox=me should return error")
	}
	if !strings.Contains(err.Error(), "不支持 mailbox=\"me\"") {
		t.Errorf("error = %v", err)
	}
	if networkCalled {
		t.Error("before network: 不应发起任何网络请求")
	}
}

// TestMailRead_AsBotWithExplicitMailbox 验证 Mail 读操作在 --as bot 指定具体邮箱时正常以 Tenant Token 访问
func TestMailRead_AsBotWithExplicitMailbox(t *testing.T) {
	var gotAuthHeader string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t-mock-tenant-token","expire":7200}`)
			return
		}
		gotAuthHeader = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	_ = mailTriageCmd.Flags().Set("as", "bot")
	_ = mailTriageCmd.Flags().Set("mailbox", "shared@example.com")
	_ = mailTriageCmd.Flags().Set("user-access-token", "")
	err := mailTriageCmd.RunE(mailTriageCmd, []string{})
	if err != nil {
		t.Fatalf("mail triage --as bot with explicit mailbox error: %v", err)
	}

	if !strings.HasPrefix(gotAuthHeader, "Bearer t-") {
		t.Errorf("auth header = %q, want Bearer t-...", gotAuthHeader)
	}
	if !strings.Contains(gotPath, "shared@example.com") {
		t.Errorf("path = %q, want containing shared@example.com", gotPath)
	}
}

// TestMailRead_AsUserWithMailboxMe 验证 Mail 读操作在 --as user 时支持 mailbox=me 并使用 User Token
func TestMailRead_AsUserWithMailboxMe(t *testing.T) {
	var gotAuthHeader string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	_ = mailTriageCmd.Flags().Set("as", "user")
	_ = mailTriageCmd.Flags().Set("mailbox", "me")
	_ = mailTriageCmd.Flags().Set("user-access-token", "u-explicit-user-token")
	err := mailTriageCmd.RunE(mailTriageCmd, []string{})
	if err != nil {
		t.Fatalf("mail triage --as user with mailbox=me error: %v", err)
	}

	if gotAuthHeader != "Bearer u-explicit-user-token" {
		t.Errorf("auth header = %q, want 'Bearer u-explicit-user-token'", gotAuthHeader)
	}
	if !strings.Contains(gotPath, "/open-apis/mail/v1/user_mailboxes/me/messages") {
		t.Errorf("path = %q, want me", gotPath)
	}
}

// TestAttendanceUserTask_AsBotAndUser 验证 attendance user-task query 的 --as bot 与 --as user 以及 corrupt token fail-closed
func TestAttendanceUserTask_AsBotAndUser(t *testing.T) {
	var gotAuthHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t-att-tenant","expire":7200}`)
			return
		}
		gotAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"user_task_results":[]}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	// 1. --as bot + 指定工号
	_ = attendanceUserTaskQueryCmd.Flags().Set("as", "bot")
	_ = attendanceUserTaskQueryCmd.Flags().Set("employee-type", "employee_no")
	_ = attendanceUserTaskQueryCmd.Flags().Set("user-ids", "10001")
	_ = attendanceUserTaskQueryCmd.Flags().Set("start", "2026-05-01")
	_ = attendanceUserTaskQueryCmd.Flags().Set("end", "2026-05-18")
	_ = attendanceUserTaskQueryCmd.Flags().Set("user-access-token", "")

	err := attendanceUserTaskQueryCmd.RunE(attendanceUserTaskQueryCmd, []string{})
	if err != nil {
		t.Fatalf("attendance user-task query --as bot error: %v", err)
	}
	if !strings.HasPrefix(gotAuthHeader, "Bearer t-") {
		t.Errorf("auth header = %q, want 'Bearer t-...'", gotAuthHeader)
	}

	// 2. --as user + 本人自查（无需传 user-ids）
	gotAuthHeader = ""
	_ = attendanceUserTaskQueryCmd.Flags().Set("as", "user")
	_ = attendanceUserTaskQueryCmd.Flags().Set("user-ids", "")
	_ = attendanceUserTaskQueryCmd.Flags().Set("employee-type", "employee_no")
	_ = attendanceUserTaskQueryCmd.Flags().Set("user-access-token", "u-att-user")

	err = attendanceUserTaskQueryCmd.RunE(attendanceUserTaskQueryCmd, []string{})
	if err != nil {
		t.Fatalf("attendance user-task query --as user error: %v", err)
	}
	if gotAuthHeader != "Bearer u-att-user" {
		t.Errorf("auth header = %q, want 'Bearer u-att-user'", gotAuthHeader)
	}

	// 3. corrupt-token 测试：auto / user 模式下若配置了非法 token 文件或 refresh 失败，必须 fail-closed 报错，绝不静默切 Bot
	os.Setenv("FEISHU_PROFILE", "corrupt_profile_not_exist")
	_ = attendanceUserTaskQueryCmd.Flags().Set("as", "user")
	_ = attendanceUserTaskQueryCmd.Flags().Set("user-access-token", "")
	err = attendanceUserTaskQueryCmd.RunE(attendanceUserTaskQueryCmd, []string{})
	if err == nil {
		t.Fatal("corrupt / missing user token under --as user must return error (fail-closed)")
	}
	os.Unsetenv("FEISHU_PROFILE")
}

// TestMailMessagesCmd_51PlusAndDuplicates 验证 mail messages 支持 51+ 条 ID 自动 20 分块且严格保留重复与顺序
func TestMailMessagesCmd_51PlusAndDuplicates(t *testing.T) {
	callCount := 0
	var requestedBatches [][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			MessageIDs []string `json:"message_ids"`
		}
		_ = json.Unmarshal(raw, &body)
		requestedBatches = append(requestedBatches, body.MessageIDs)

		var msgs []map[string]any
		for _, id := range body.MessageIDs {
			msgs = append(msgs, map[string]any{
				"message_id": id,
				"subject":    "Subject of " + id,
			})
		}
		respData := map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{
				"messages": msgs,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(respData)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	// 构造 55 个 ID，包含重复项
	var ids []string
	for i := 1; i <= 53; i++ {
		ids = append(ids, fmt.Sprintf("msg_%02d", i))
	}
	ids = append(ids, "msg_01", "msg_02") // 54, 55 (重复 msg_01, msg_02)
	rawCSV := strings.Join(ids, ",")

	_ = mailMessagesCmd.Flags().Set("as", "user")
	_ = mailMessagesCmd.Flags().Set("mailbox", "me")
	_ = mailMessagesCmd.Flags().Set("message-ids", rawCSV)
	_ = mailMessagesCmd.Flags().Set("user-access-token", "u-test-token")

	err := mailMessagesCmd.RunE(mailMessagesCmd, []string{})
	if err != nil {
		t.Fatalf("mail messages 55 items error: %v", err)
	}

	// 验证网络发出了 3 批（20, 20, 15）
	if callCount != 3 {
		t.Errorf("55 条 ID 应分 3 批网络请求，实际 %d 批", callCount)
	}
	if len(requestedBatches) != 3 || len(requestedBatches[0]) != 20 || len(requestedBatches[1]) != 20 || len(requestedBatches[2]) != 15 {
		t.Errorf("网络分块大小异常: %v", requestedBatches)
	}
}

// TestMailMessagesCmd_EmptySegmentsRejected 验证空 segment（如 "m1,,m2"）在发起网络请求前被拒绝报错
func TestMailMessagesCmd_EmptySegmentsRejected(t *testing.T) {
	networkCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		networkCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	invalidCSVs := []string{
		"m1,,m2",
		",m1",
		"m1,",
		"m1, ,m2",
		"m1,   ",
		"",
		"   ",
	}

	for _, csv := range invalidCSVs {
		networkCalled = false
		_ = mailMessagesCmd.Flags().Set("as", "user")
		_ = mailMessagesCmd.Flags().Set("mailbox", "me")
		_ = mailMessagesCmd.Flags().Set("message-ids", csv)
		_ = mailMessagesCmd.Flags().Set("format", "full")
		_ = mailMessagesCmd.Flags().Set("user-access-token", "u-test-token")

		err := mailMessagesCmd.RunE(mailMessagesCmd, []string{})
		if err == nil {
			t.Errorf("输入 %q 应报错，但通过了", csv)
		}
		if networkCalled {
			t.Errorf("输入 %q 时不应触发网络调用", csv)
		}
	}
}

// TestMailMessagesCmd_PreflightValidationZeroNetworkAndTokenRefresh 验证在配置了损坏/过期 token 的 auto 模式下，本地参数校验前置，非法输入零网络、零 token 写
func TestMailMessagesCmd_PreflightValidationZeroNetworkAndTokenRefresh(t *testing.T) {
	networkCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		networkCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	// 配置 corrupt profile 模拟损坏或过期 token
	os.Setenv("FEISHU_PROFILE", "corrupt_profile_stale")
	defer os.Unsetenv("FEISHU_PROFILE")

	// 1. 空 segment 校验：应在本地报错，零网络
	networkCalled = false
	_ = mailMessagesCmd.Flags().Set("as", "auto")
	_ = mailMessagesCmd.Flags().Set("mailbox", "me")
	_ = mailMessagesCmd.Flags().Set("message-ids", "msg_1,,msg_2")
	_ = mailMessagesCmd.Flags().Set("format", "full")
	_ = mailMessagesCmd.Flags().Set("user-access-token", "")

	err := mailMessagesCmd.RunE(mailMessagesCmd, []string{})
	if err == nil {
		t.Fatal("空 segment 必须报错")
	}
	if !strings.Contains(err.Error(), "包含空的邮件 ID") {
		t.Errorf("error = %v, want 本地参数校验错误", err)
	}
	if networkCalled {
		t.Error("非法 CSV 时不应发起任何网络调用或触发 token 刷新")
	}

	// 2. 非法 format 校验（服务端只接受 full/plain_text_full/metadata，raw 不支持）：应在本地报错，零网络
	networkCalled = false
	_ = mailMessagesCmd.Flags().Set("as", "auto")
	_ = mailMessagesCmd.Flags().Set("mailbox", "me")
	_ = mailMessagesCmd.Flags().Set("message-ids", "msg_1,msg_2")
	_ = mailMessagesCmd.Flags().Set("format", "raw")
	_ = mailMessagesCmd.Flags().Set("output", "")
	_ = mailMessagesCmd.Flags().Set("user-access-token", "")

	err = mailMessagesCmd.RunE(mailMessagesCmd, []string{})
	if err == nil {
		t.Fatal("非法 format 必须报错")
	}
	if !strings.Contains(err.Error(), "--format 仅支持 full / plain_text_full / metadata") {
		t.Errorf("error = %v, want format 校验错误", err)
	}
	if exitCodeFor(err) != 2 {
		t.Errorf("非法 format 应为用法错误 exit 2，实际 %d", exitCodeFor(err))
	}
	if networkCalled {
		t.Error("非法 format 时不应发起任何网络调用或触发 token 刷新")
	}

	// 3. 非法 output 校验（如 output=yaml）：应在本地报错，零网络
	networkCalled = false
	_ = mailMessagesCmd.Flags().Set("as", "auto")
	_ = mailMessagesCmd.Flags().Set("mailbox", "me")
	_ = mailMessagesCmd.Flags().Set("message-ids", "msg_1,msg_2")
	_ = mailMessagesCmd.Flags().Set("format", "full")
	_ = mailMessagesCmd.Flags().Set("output", "yaml")
	_ = mailMessagesCmd.Flags().Set("user-access-token", "")

	err = mailMessagesCmd.RunE(mailMessagesCmd, []string{})
	if err == nil {
		t.Fatal("非法 output 必须报错")
	}
	if !strings.Contains(err.Error(), "output 仅支持 json") {
		t.Errorf("error = %v, want output 校验错误", err)
	}
	if networkCalled {
		t.Error("非法 output 时不应发起任何网络调用或触发 token 刷新")
	}
}
