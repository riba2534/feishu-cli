package client

import "testing"

// TestBuildVCBotJoinBody 锁住 B1 修复：机器人入会请求体必须含 join_type(=整数1) 与
// join_identify(={meeting_no})，password 在顶层而非嵌进 join_identify。
// 结构与服务端契约一致（已实测）——
// 之前只发 {meeting_no,password} 被 server 拒为 99992402 field validation failed。
func TestBuildVCBotJoinBody(t *testing.T) {
	t.Run("无密码", func(t *testing.T) {
		body := BuildVCBotJoinBody(VCBotJoinReq{MeetingNo: "123456789"})

		if body["join_type"] != 1 {
			t.Errorf("join_type = %v(%T), want 整数 1", body["join_type"], body["join_type"])
		}
		ident, ok := body["join_identify"].(map[string]any)
		if !ok {
			t.Fatalf("join_identify 应为对象，实为 %T", body["join_identify"])
		}
		if ident["meeting_no"] != "123456789" {
			t.Errorf("join_identify.meeting_no = %v, want 123456789", ident["meeting_no"])
		}
		if _, has := body["password"]; has {
			t.Errorf("未传 password 时不应出现 password 字段")
		}
	})

	t.Run("有密码", func(t *testing.T) {
		body := BuildVCBotJoinBody(VCBotJoinReq{MeetingNo: "987654321", Password: "8888"})

		if body["password"] != "8888" {
			t.Errorf("password 应在顶层 = 8888，实为 %v", body["password"])
		}
		ident, _ := body["join_identify"].(map[string]any)
		if _, has := ident["meeting_no"]; !has {
			t.Errorf("join_identify 应含 meeting_no")
		}
		if _, has := ident["password"]; has {
			t.Errorf("password 不应嵌进 join_identify（必须在顶层）")
		}
	})
}

// TestBuildVCBotJoinBodyCallIDAndStart 对齐官方 vc_meeting_join.go：call_id 顶层透传，
// --action start 对应 action=2；默认（加入）不发送 action。
func TestBuildVCBotJoinBodyCallIDAndStart(t *testing.T) {
	body := BuildVCBotJoinBody(VCBotJoinReq{MeetingNo: "123456789", CallID: "call-1", Start: true})
	if body["call_id"] != "call-1" {
		t.Errorf("call_id = %v", body["call_id"])
	}
	if body["action"] != 2 {
		t.Errorf("action = %v(%T), want 2", body["action"], body["action"])
	}
	plain := BuildVCBotJoinBody(VCBotJoinReq{MeetingNo: "123456789"})
	if _, has := plain["action"]; has {
		t.Errorf("默认加入不应发送 action")
	}
	if _, has := plain["call_id"]; has {
		t.Errorf("未传 call_id 不应发送")
	}
}
