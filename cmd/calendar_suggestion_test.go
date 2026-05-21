package cmd

import "testing"

// TestCalendarSuggestionCmdRegistered 验证 suggestion 子命令挂到 calendarCmd
func TestCalendarSuggestionCmdRegistered(t *testing.T) {
	if calendarSuggestionCmd.Use != "suggestion" {
		t.Fatalf("Use = %q, want suggestion", calendarSuggestionCmd.Use)
	}
	if calendarSuggestionCmd.Short == "" {
		t.Fatal("Short should not be empty")
	}
	found := false
	for _, sub := range calendarCmd.Commands() {
		if sub == calendarSuggestionCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("calendarSuggestionCmd should be registered as child of calendarCmd")
	}
}

// TestCalendarSuggestionFlagsDefaults 验证 flag 注册
func TestCalendarSuggestionFlagsDefaults(t *testing.T) {
	want := []string{"attendee-ids", "duration", "start", "end", "timezone", "event-rrule", "exclude", "output", "user-access-token"}
	for _, name := range want {
		if calendarSuggestionCmd.Flags().Lookup(name) == nil {
			t.Errorf("--%s missing", name)
		}
	}
	if out := calendarSuggestionCmd.Flags().Lookup("output"); out != nil && out.Shorthand != "o" {
		t.Errorf("--output shorthand=%q, want o", out.Shorthand)
	}
}

// TestCalendarSuggestionRequiredFlags 验证 attendee-ids/duration 必填
func TestCalendarSuggestionRequiredFlags(t *testing.T) {
	for _, name := range []string{"attendee-ids", "duration"} {
		f := calendarSuggestionCmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("--%s missing", name)
		}
		ann := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]
		if len(ann) == 0 || ann[0] != "true" {
			t.Errorf("--%s should be required, ann=%v", name, ann)
		}
	}
}
