package client

import (
	"net/http"
	"testing"
)

// TestApprovalBusinessErrorOn400 审批业务错误随 HTTP 400 下发时按业务码返回 APIError（不再只给 HTTP 400 body）
func TestApprovalBusinessErrorOn400(t *testing.T) {
	captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusBadRequest, `{"code":1390001,"msg":"param is invalid"}`)
	})
	_, err := QueryApprovalTasks(ApprovalTaskQueryOptions{Topic: "1"}, testUserToken)
	if apiErr, ok := AsAPIError(err); !ok || apiErr.Code != 1390001 {
		t.Fatalf("期望 APIError code=1390001，得到 %v", err)
	}
}

func TestBuildAddSignApprovalTaskBody(t *testing.T) {
	cases := []struct {
		name    string
		opts    AddSignApprovalTaskOptions
		wantErr bool
		method  any
	}{
		{"单人前加签默认或签", AddSignApprovalTaskOptions{InstanceCode: "ic", TaskID: "t", AddSignType: 1, AddSignUserIDs: []string{"ou_a"}}, false, 1},
		{"多人前加签必须指定方式", AddSignApprovalTaskOptions{InstanceCode: "ic", TaskID: "t", AddSignType: 1, AddSignUserIDs: []string{"ou_a", "ou_b"}}, true, nil},
		{"多人后加签会签", AddSignApprovalTaskOptions{InstanceCode: "ic", TaskID: "t", AddSignType: 2, AddSignUserIDs: []string{"ou_a", "ou_b"}, ApprovalMethod: 2}, false, 2},
		{"并加签不传方式", AddSignApprovalTaskOptions{InstanceCode: "ic", TaskID: "t", AddSignType: 3, AddSignUserIDs: []string{"ou_a"}}, false, nil},
		{"并加签带方式报错", AddSignApprovalTaskOptions{InstanceCode: "ic", TaskID: "t", AddSignType: 3, AddSignUserIDs: []string{"ou_a"}, ApprovalMethod: 1}, true, nil},
		{"非法类型", AddSignApprovalTaskOptions{InstanceCode: "ic", TaskID: "t", AddSignType: 4, AddSignUserIDs: []string{"ou_a"}}, true, nil},
	}
	for _, c := range cases {
		body, idType, err := BuildAddSignApprovalTaskBody(c.opts)
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: err=%v wantErr=%v", c.name, err, c.wantErr)
		}
		if err != nil {
			continue
		}
		if idType != "open_id" || body["approval_method"] != c.method {
			t.Fatalf("%s: idType=%s method=%v want %v", c.name, idType, body["approval_method"], c.method)
		}
	}
}

func TestApprovalTaskMoreEndpoints(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"data":{}}`)
	})
	if err := RollbackApprovalTask(RollbackApprovalTaskOptions{InstanceCode: "ic", TaskID: "t", NodeIDs: []string{"START"}}, testUserToken); err != nil {
		t.Fatal(err)
	}
	if err := RemindApprovalTask(RemindApprovalTaskOptions{InstanceCode: "ic", TaskIDs: []string{"t"}}, testUserToken); err != nil {
		t.Fatal(err)
	}
	if err := AddSignApprovalTask(AddSignApprovalTaskOptions{InstanceCode: "ic", TaskID: "t", AddSignType: 3, AddSignUserIDs: []string{"ou_a"}}, testUserToken); err != nil {
		t.Fatal(err)
	}
	reqs := got()
	want := []string{"/open-apis/approval/v4/tasks/rollback", "/open-apis/approval/v4/instances/remind", "/open-apis/approval/v4/tasks/add_sign"}
	for i, w := range want {
		if reqs[i].Path != w || reqs[i].Auth != "Bearer "+testUserToken {
			t.Fatalf("请求 %d = %s %s，期望 %s（User Token）", i, reqs[i].Path, reqs[i].Auth, w)
		}
	}
	if reqs[2].Query.Get("user_id_type") != "open_id" {
		t.Fatalf("add_sign 应带 user_id_type: %v", reqs[2].Query)
	}
	if _, err := BuildRollbackApprovalTaskBody(RollbackApprovalTaskOptions{InstanceCode: "ic", TaskID: "t"}); err == nil {
		t.Fatal("缺 node_ids 应报错")
	}
}
