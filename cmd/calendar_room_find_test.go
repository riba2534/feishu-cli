package cmd

import "testing"

// TestCalendarRoomFindCmdRegistered 验证 room-find 子命令注册
func TestCalendarRoomFindCmdRegistered(t *testing.T) {
	if calendarRoomFindCmd.Use != "room-find" {
		t.Fatalf("Use = %q, want room-find", calendarRoomFindCmd.Use)
	}
	if calendarRoomFindCmd.Short == "" {
		t.Fatal("Short should not be empty")
	}
	found := false
	for _, sub := range calendarCmd.Commands() {
		if sub == calendarRoomFindCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("calendarRoomFindCmd should be child of calendarCmd")
	}
}

// TestCalendarRoomFindFlagsDefaults 验证 flag 注册
func TestCalendarRoomFindFlagsDefaults(t *testing.T) {
	wantString := []string{"slot", "attendee-ids", "city", "building", "floor", "room-name", "timezone", "event-rrule", "output", "user-access-token"}
	for _, n := range wantString {
		if calendarRoomFindCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing", n)
		}
	}
	for _, n := range []string{"min-capacity", "max-capacity"} {
		f := calendarRoomFindCmd.Flags().Lookup(n)
		if f == nil {
			t.Errorf("--%s missing", n)
			continue
		}
		if f.DefValue != "0" {
			t.Errorf("--%s default=%q, want 0", n, f.DefValue)
		}
	}
}

// TestCalendarRoomFindSlotIsStringSlice 验证 slot stringSlice 类型
func TestCalendarRoomFindSlotIsStringSlice(t *testing.T) {
	f := calendarRoomFindCmd.Flags().Lookup("slot")
	if f == nil {
		t.Fatal("--slot missing")
	}
	if f.Value.Type() != "stringSlice" {
		t.Errorf("--slot type=%q, want stringSlice", f.Value.Type())
	}
}
