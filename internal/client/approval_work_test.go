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
