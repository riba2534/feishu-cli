package client

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildCreateApprovalInstanceBody_FieldMapping 直接断言 body 字段名称 +
// 返回的 normUIDType（builder 内部 normalize 结果，与 body 字段映射对齐）：
//   - open_id / 空 UserIDType 一律落 body["open_id"]（normalize）+ normUIDType=="open_id"
//   - user_id 落 body["user_id"] + normUIDType=="user_id"
//   - 不存在的字段名（如 union_id）不出现
//   - 含空白的 UserIDType 也得 trim 干净
//
// 注意：normUIDType **不用于拼 query** —— /approval/v4/instances 端点 body 字段区分身份，
// SDK CreateInstanceReqBuilder 不暴露 UserIdType()。CreateApprovalInstance 实际调
// doApprovalPost(..., "", ...) 传空字符串。此处仍返回 normUIDType 是为错误信息 +
// 单测断言 normalize 行为 + 未来 endpoint 扩展可能。
func TestBuildCreateApprovalInstanceBody_FieldMapping(t *testing.T) {
	cases := []struct {
		name        string
		uidType     string
		wantField   string // body 中应等于 opts.UserID 的字段
		absentField string // body 中应缺失的字段
		wantNormUID string // builder 返回的 normalized UIDType（不拼 query，见函数注释）
	}{
		{"empty defaults to open_id", "", "open_id", "user_id", "open_id"},
		{"explicit open_id", "open_id", "open_id", "user_id", "open_id"},
		{"explicit user_id", "user_id", "user_id", "open_id", "user_id"},
		{"whitespace open_id trimmed", "  open_id  ", "open_id", "user_id", "open_id"},
		{"whitespace user_id trimmed", "\tuser_id\n", "user_id", "open_id", "user_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, normUID, err := buildCreateApprovalInstanceBody(CreateApprovalInstanceOptions{
				ApprovalCode: "AC-1",
				UserID:       "ou_xxx",
				Form:         "[]",
				UserIDType:   tc.uidType,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got, ok := body[tc.wantField]; !ok || got != "ou_xxx" {
				t.Errorf("body[%q] = %v ok=%v, want \"ou_xxx\"", tc.wantField, got, ok)
			}
			if _, ok := body[tc.absentField]; ok {
				t.Errorf("body[%q] should be absent, got %v", tc.absentField, body[tc.absentField])
			}
			if _, ok := body["union_id"]; ok {
				t.Errorf("body should never contain union_id field")
			}
			if normUID != tc.wantNormUID {
				t.Errorf("normUIDType = %q, want %q (body/query 不一致会导致 HTTP 请求两端 user_id_type 漂移)", normUID, tc.wantNormUID)
			}
		})
	}
}

// TestBuildCreateApprovalInstanceBody_RejectsUnionID 验证 endpoint 不支持的 union_id 被显式拒绝。
// 早期实现把 union_id 错误映射到 user_id 字段（飞书 SDK InstanceCreate struct 不含 union_id 字段）。
func TestBuildCreateApprovalInstanceBody_RejectsUnionID(t *testing.T) {
	_, _, err := buildCreateApprovalInstanceBody(CreateApprovalInstanceOptions{
		ApprovalCode: "AC-1",
		UserID:       "on_xxx",
		Form:         "[]",
		UserIDType:   "union_id",
	})
	if err == nil {
		t.Fatal("expected union_id to be rejected, got nil error")
	}
	if !strings.Contains(err.Error(), "union_id") {
		t.Errorf("error should mention union_id, got: %v", err)
	}
	if !strings.Contains(err.Error(), "open_id") || !strings.Contains(err.Error(), "user_id") {
		t.Errorf("error should list valid alternatives (open_id / user_id), got: %v", err)
	}
}

// TestBuildCreateApprovalInstanceBody_RejectsInvalid 验证完全乱写的 user_id_type 也被拒绝。
func TestBuildCreateApprovalInstanceBody_RejectsInvalid(t *testing.T) {
	_, _, err := buildCreateApprovalInstanceBody(CreateApprovalInstanceOptions{
		ApprovalCode: "AC-1",
		UserID:       "ou_xxx",
		Form:         "[]",
		UserIDType:   "bogus_type",
	})
	if err == nil {
		t.Fatal("expected error for invalid user_id_type")
	}
	if !strings.Contains(err.Error(), "user_id_type") {
		t.Errorf("expected user_id_type error, got: %v", err)
	}
}

// TestBuildCreateApprovalInstanceBody_RequiredFields 验证三个必填校验。
func TestBuildCreateApprovalInstanceBody_RequiredFields(t *testing.T) {
	base := CreateApprovalInstanceOptions{
		ApprovalCode: "AC-1",
		UserID:       "ou_xxx",
		Form:         "[]",
	}
	cases := []struct {
		name   string
		mutate func(*CreateApprovalInstanceOptions)
		want   string
	}{
		{"missing approval_code", func(o *CreateApprovalInstanceOptions) { o.ApprovalCode = "" }, "approval_code"},
		{"whitespace approval_code", func(o *CreateApprovalInstanceOptions) { o.ApprovalCode = "   " }, "approval_code"},
		{"missing user_id", func(o *CreateApprovalInstanceOptions) { o.UserID = "" }, "user_id"},
		{"missing form", func(o *CreateApprovalInstanceOptions) { o.Form = "" }, "form"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := base
			tc.mutate(&opts)
			_, _, err := buildCreateApprovalInstanceBody(opts)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected error containing %q, got: %v", tc.want, err)
			}
		})
	}
}

// TestBuildCreateApprovalInstanceBody_NodeListsParsed 验证 NodeApprover/NodeCC 原文 JSON 被解析进 body。
func TestBuildCreateApprovalInstanceBody_NodeListsParsed(t *testing.T) {
	body, _, err := buildCreateApprovalInstanceBody(CreateApprovalInstanceOptions{
		ApprovalCode:           "AC-1",
		UserID:                 "ou_xxx",
		Form:                   "[]",
		UserIDType:             "open_id",
		NodeApproverUserIDList: json.RawMessage(`[{"node_id":"n1","value":["ou_a"]}]`),
		NodeCCUserIDList:       json.RawMessage(`[{"node_id":"n2","value":["ou_b"]}]`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := body["node_approver_user_id_list"]; !ok {
		t.Error("node_approver_user_id_list should be present")
	}
	if _, ok := body["node_cc_user_id_list"]; !ok {
		t.Error("node_cc_user_id_list should be present")
	}
}

// TestCreateApprovalInstance_RequiredFieldsRegression 外层入口的必填校验回归（不打 HTTP）。
// 早期名 TestCreateApprovalInstanceNormalizesEmptyUserIDType 称 normalize 测试但实际只触发
// 必填短路、根本没走到 body/query 构造。重命名为 RequiredFieldsRegression 反映真实意图。
func TestCreateApprovalInstance_RequiredFieldsRegression(t *testing.T) {
	_, err := CreateApprovalInstance(CreateApprovalInstanceOptions{
		ApprovalCode: "",
		UserID:       "ou_xxx",
		Form:         "[]",
		UserIDType:   "",
	}, "")
	if err == nil {
		t.Fatal("expected error for empty approval_code")
	}
	if !strings.Contains(err.Error(), "approval_code") {
		t.Errorf("expected approval_code error, got: %v", err)
	}
}

// TestCreateApprovalInstance_RejectsInvalidUserIDType 外层入口的非法 user_id_type 回归（不打 HTTP）。
func TestCreateApprovalInstance_RejectsInvalidUserIDType(t *testing.T) {
	_, err := CreateApprovalInstance(CreateApprovalInstanceOptions{
		ApprovalCode: "OK",
		UserID:       "ou_xxx",
		Form:         "[]",
		UserIDType:   "bogus_type",
	}, "")
	if err == nil {
		t.Fatal("expected error for invalid user_id_type")
	}
	if !strings.Contains(err.Error(), "user_id_type") {
		t.Errorf("expected user_id_type error, got: %v", err)
	}
}
