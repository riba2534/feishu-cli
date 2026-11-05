package cmd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// TestOKRAdjustDryRunNoCredentials 新增 OKR 写命令 --dry-run 无凭据 exit 0、不联网
func TestOKRAdjustDryRunNoCredentials(t *testing.T) {
	cases := []struct {
		cmd   *cobra.Command
		args  []string
		flags map[string]string
		want  []string
	}{
		{okrWeightCmd, nil, map[string]string{"level": "objective", "cycle-id": "100", "weights": `[{"id":"7a","weight":0.4}]`}, []string{"/open-apis/okr/v2/cycles/100/objectives", "/open-apis/okr/v2/cycles/100/objectives_weight", `"objective_id": "7a"`}},
		{okrWeightCmd, nil, map[string]string{"level": "key-result", "objective-id": "200", "weights": `[{"id":"k1","weight":0.5}]`}, []string{"/open-apis/okr/v2/objectives/200/key_results_weight", `"key_result_id": "k1"`}},
		{okrReorderCmd, nil, map[string]string{"level": "key-result", "objective-id": "200", "ops": `[{"id":"k2","position":1}]`}, []string{"/open-apis/okr/v2/objectives/200/key_results_position", `"key_result_ids"`}},
		{okrIndicatorUpdateCmd, nil, map[string]string{"level": "key-result", "id": "300", "value": "42"}, []string{"/open-apis/okr/v2/key_results/300/indicators", `"current_value": 42`}},
		{okrObjectiveBatchCreateCmd, nil, map[string]string{"cycle-id": "100", "input": `[{"text":"O1","krs":[{"text":"K1"}]}]`}, []string{"/open-apis/okr/v2/cycles/100/objectives", "/open-apis/okr/v2/objectives/<objective_id>/key_results", "回滚"}},
		{okrCommentUpdateCmd, []string{"400"}, map[string]string{"content": "改"}, []string{`"PATCH"`, "/open-apis/okr/v2/comments/400"}},
		{okrCommentDeleteCmd, []string{"400"}, nil, []string{`"DELETE"`, "/open-apis/okr/v2/comments/400"}},
	}
	for _, c := range cases {
		rec := setupNoCredsWorkTest(t)
		resetCommandTreeFlags(okrCmd)
		flags := map[string]string{"dry-run": "true"}
		for k, v := range c.flags {
			flags[k] = v
		}
		out, err := runWorkCmd(t, c.cmd, c.args, flags)
		if err != nil {
			t.Fatalf("%s dry-run: %v", c.cmd.CommandPath(), err)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Fatalf("%s dry-run 缺少 %q:\n%s", c.cmd.CommandPath(), w, out)
			}
		}
		if len(rec.all()) != 0 {
			t.Fatalf("%s dry-run 不应联网: %+v", c.cmd.CommandPath(), rec.all())
		}
	}
}

// TestOKRAdjustParamErrors 参数错误 exit 2 且不联网
func TestOKRAdjustParamErrors(t *testing.T) {
	cases := []struct {
		cmd   *cobra.Command
		args  []string
		flags map[string]string
	}{
		{okrWeightCmd, nil, map[string]string{"level": "x", "cycle-id": "1", "weights": `[{"id":"a","weight":0.1}]`}},
		{okrWeightCmd, nil, map[string]string{"level": "objective", "cycle-id": "abc", "weights": `[{"id":"a","weight":0.1}]`}},
		{okrWeightCmd, nil, map[string]string{"level": "key-result", "weights": `[{"id":"a","weight":0.1}]`}},
		{okrWeightCmd, nil, map[string]string{"level": "objective", "cycle-id": "1", "weights": `[{"id":"a","weight":0.6},{"id":"b","weight":0.6}]`}},
		{okrWeightCmd, nil, map[string]string{"level": "objective", "cycle-id": "1", "weights": `[{"id":"a","weight":0.1234}]`}},
		{okrWeightCmd, nil, map[string]string{"level": "objective", "cycle-id": "1", "weights": `[{"id":"a","weight":0.1},{"id":"a","weight":0.2}]`}},
		{okrReorderCmd, nil, map[string]string{"level": "objective", "cycle-id": "1", "ops": `[{"id":"a","position":0}]`}},
		{okrIndicatorUpdateCmd, nil, map[string]string{"level": "objective", "id": "1", "value": "abc"}},
		{okrObjectiveBatchCreateCmd, nil, map[string]string{"cycle-id": "1", "input": `[]`}},
		{okrCommentUpdateCmd, []string{"1"}, nil},
		{okrCommentGetCmd, []string{"x"}, nil},
	}
	for _, c := range cases {
		rec := setupNoCredsWorkTest(t)
		resetCommandTreeFlags(okrCmd)
		_, err := runWorkCmd(t, c.cmd, c.args, c.flags)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("%s %v 期望用法错误，得到 %v", c.cmd.CommandPath(), c.flags, err)
		}
		if len(rec.all()) != 0 {
			t.Fatalf("不应联网: %+v", rec.all())
		}
	}
}

// TestOKRWeightExecutesNormalizedPut 真实路径：读取现有目标后 PUT 归一化的完整列表
func TestOKRWeightExecutesNormalizedPut(t *testing.T) {
	rec := setupWorkCmdTest(t, "", func(w http.ResponseWriter, r *http.Request, body string) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"o2","position":2,"weight":0.5},{"id":"o1","position":1,"weight":0.5}],"has_more":false}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	})
	resetCommandTreeFlags(okrCmd)
	out, err := runWorkCmd(t, okrWeightCmd, nil, map[string]string{"level": "objective", "cycle-id": "100", "weights": `[{"id":"o2","weight":0.7}]`, "output": "json"})
	if err != nil {
		t.Fatalf("okr weight: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 2 || reqs[1].Method != http.MethodPut || reqs[1].Path != "/open-apis/okr/v2/cycles/100/objectives_weight" {
		t.Fatalf("请求不对: %+v", reqs)
	}
	if reqs[1].Body != `{"objective_weights":[{"objective_id":"o1","weight":0.300},{"objective_id":"o2","weight":0.700}]}` {
		t.Fatalf("PUT 请求体应为按 position 排序的完整列表: %s", reqs[1].Body)
	}
	if reqs[0].Auth != "Bearer t-bot" {
		t.Fatalf("okr 默认 Bot 身份: %+v", reqs[0])
	}
	if !strings.Contains(out, `"total": 2`) {
		t.Fatalf("输出不对: %s", out)
	}
}

// TestOKRCommentDeleteGates 删除评论必需 User Token；非交互未确认 exit 10
func TestOKRCommentDeleteGates(t *testing.T) {
	rec := setupWorkCmdTest(t, "", nil)
	resetCommandTreeFlags(okrCmd)
	if _, err := runWorkCmd(t, okrCommentDeleteCmd, []string{"400"}, nil); err == nil || !strings.Contains(err.Error(), "User") {
		t.Fatalf("未登录应要求 User Token: %v", err)
	}
	if len(rec.apiReqs()) != 0 {
		t.Fatalf("不应发请求: %+v", rec.apiReqs())
	}
	rec = setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	})
	resetCommandTreeFlags(okrCmd)
	_, err := runWorkCmd(t, okrCommentDeleteCmd, []string{"400"}, nil)
	if err == nil || !clierr.HasKind(err, clierr.KindConfirmationRequired) {
		t.Fatalf("期望需要确认，得到 %v", err)
	}
	if len(rec.apiReqs()) != 0 {
		t.Fatalf("未确认不应删除: %+v", rec.apiReqs())
	}
}
