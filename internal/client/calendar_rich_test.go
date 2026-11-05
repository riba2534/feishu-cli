package client

import (
	"testing"

	larkcalendar "github.com/larksuite/oapi-sdk-go/v3/service/calendar/v4"
)

// TestBuildEventBodyWithRich description_rich 替换 description，其余字段保留
func TestBuildEventBodyWithRich(t *testing.T) {
	ev := larkcalendar.NewCalendarEventBuilder().
		Summary("s").
		Description("plain").
		Vchat(larkcalendar.NewVchatBuilder().VcType("vc").MeetingSettings(larkcalendar.NewMeetingSettingsBuilder().OwnerId("ou_a").Build()).Build()).
		Build()
	body, err := BuildEventBodyWithRich(ev, "**rich**")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["description"]; ok {
		t.Fatalf("description 应被移除: %+v", body)
	}
	if body["description_rich"] != "**rich**" || body["summary"] != "s" {
		t.Fatalf("请求体不对: %+v", body)
	}
	ms := body["vchat"].(map[string]any)["meeting_settings"].(map[string]any)
	if ms["owner_id"] != "ou_a" {
		t.Fatalf("meeting_settings 丢失: %+v", body["vchat"])
	}
}

// TestCalendarImagePreviewURL 预览 URL 带尺寸与大小
func TestCalendarImagePreviewURL(t *testing.T) {
	got := CalendarImagePreviewURL("img.example.com", "boxcnxxx", 10, 20, 30)
	want := "https://img.example.com/space/api/box/stream/download/preview/boxcnxxx?preview_type=16&im_w=10&im_h=20&im_size=30"
	if got != want {
		t.Fatalf("got %s", got)
	}
	if got := CalendarImagePreviewURL("h", "t", 0, 0, 0); got != "https://h/space/api/box/stream/download/preview/t?preview_type=16" {
		t.Fatalf("无尺寸时不应带参数: %s", got)
	}
}

// TestUpdateEventParamsHasFieldsRich 只传富文本描述也算有更新字段
func TestUpdateEventParamsHasFieldsRich(t *testing.T) {
	p := UpdateEventParams{DescriptionRich: "x"}
	if !p.HasFields() {
		t.Fatal("DescriptionRich 应计为待更新字段")
	}
}
