package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// ---------- 仪表盘 / 分享（P1-9） ----------

func newBlockBodyTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	for _, f := range []string{"name", "type", "data-config", "position", "config", "config-file"} {
		c.Flags().String(f, "", "")
	}
	return c
}

func TestDashboardBlockBodyRankingAndPosition(t *testing.T) {
	c := newBlockBodyTestCmd()
	_ = c.Flags().Set("type", "ranking")
	if _, err := dashboardBuildBlockBody(c, true); err == nil {
		t.Error("ranking 缺 --data-config 应报错")
	}
	_ = c.Flags().Set("data-config", `{"table_name":"订单"}`)
	_ = c.Flags().Set("position", `{"x":0,"y":0,"w":6,"h":4}`)
	body, err := dashboardBuildBlockBody(c, true)
	if err != nil {
		t.Fatalf("ranking + position 应通过: %v", err)
	}
	if body["type"] != "ranking" || body["position"] == nil {
		t.Errorf("body 不对: %v", body)
	}
	c2 := newBlockBodyTestCmd()
	_ = c2.Flags().Set("type", "nps")
	_ = c2.Flags().Set("data-config", `{"table_name":"t","group_by":[{"field_name":"评分"}]}`)
	if _, err := dashboardBuildBlockBody(c2, true); err != nil {
		t.Errorf("nps 应是合法类型: %v", err)
	}
	c3 := newBlockBodyTestCmd()
	_ = c3.Flags().Set("position", `{"x":0,"y":"1"}`)
	if _, err := dashboardBuildBlockBody(c3, false); err == nil || !strings.Contains(err.Error(), "y/w/h") {
		t.Errorf("position 缺键/非数字应报错，got %v", err)
	}
}

func TestBuildBitableShareUpdateBody(t *testing.T) {
	newCmd := func() *cobra.Command {
		c := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
		c.Flags().Bool("enabled", false, "")
		c.Flags().String("access-scope", "", "")
		c.Flags().Bool("require-login", false, "")
		return c
	}
	settings := [][2]string{{"require-login", "require_login"}}
	if _, err := buildBitableShareUpdateBody(newCmd(), settings); err == nil {
		t.Error("一个字段都不传应报错")
	}
	c := newCmd()
	_ = c.Flags().Set("enabled", "true")
	_ = c.Flags().Set("access-scope", "tenant")
	if _, err := buildBitableShareUpdateBody(c, settings); err == nil || !strings.Contains(err.Error(), "每次只能修改一个") {
		t.Errorf("同时改两个字段应报错，got %v", err)
	}
	c = newCmd()
	_ = c.Flags().Set("require-login", "false")
	body, err := buildBitableShareUpdateBody(c, settings)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := body["settings"].(map[string]any); s["require_login"] != false {
		t.Errorf("--require-login=false 应显式下发 false: %v", body)
	}
	c = newCmd()
	_ = c.Flags().Set("access-scope", "public")
	if _, err := buildBitableShareUpdateBody(c, settings); err == nil {
		t.Error("非法 access-scope 应报错")
	}
}
