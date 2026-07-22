package client

import (
	"fmt"
	"testing"
)

func TestHasAPICode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		want bool
	}{
		{"标准 code= 形态", fmt.Errorf("获取失败: code=2091003, msg=生成中"), 2091003, true},
		{"包装后仍命中", fmt.Errorf("外层: %w", fmt.Errorf("code=1062507, msg=full")), 1062507, true},
		{"raw body JSON 形态", fmt.Errorf(`HTTP 400, body: {"code": 232033,"msg":"x"}`), 232033, true},
		{"log_id 同数字串不误判", fmt.Errorf(`code=99991679, msg=x, log_id=20260722091003ABC`), 2091003, false},
		{"数字是前缀不误判", fmt.Errorf("code=10625071, msg=x"), 1062507, false},
		{"nil 错误", nil, 1062507, false},
		{"无关错误", fmt.Errorf("网络超时"), 232033, false},
	}
	for _, c := range cases {
		if got := HasAPICode(c.err, c.code); got != c.want {
			t.Errorf("%s: HasAPICode(%v, %d) = %v, want %v", c.name, c.err, c.code, got, c.want)
		}
	}
}
