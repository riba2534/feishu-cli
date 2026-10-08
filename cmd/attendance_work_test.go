package cmd

import (
	"strings"
	"testing"
)

// TestAttendanceHelpExamplesUseSupportedEmployeeType 帮助示例只能用 employee_id / employee_no
// （回归：示例写 --employee-type open_id，照抄必报"不支持"）
func TestAttendanceHelpExamplesUseSupportedEmployeeType(t *testing.T) {
	for _, c := range []struct {
		name string
		long string
	}{
		{"attendance", attendanceCmd.Long},
		{"user-task", attendanceUserTaskCmd.Long},
		{"user-task query", attendanceUserTaskQueryCmd.Long},
		{"user-stats", attendanceUserStatsCmd.Long},
		{"user-stats query", attendanceUserStatsQueryCmd.Long},
	} {
		if strings.Contains(c.long, "--employee-type open_id") || strings.Contains(c.long, "--current-user-id ou_") {
			t.Fatalf("%s 帮助示例使用了不支持的 open_id: %s", c.name, c.long)
		}
	}
}
