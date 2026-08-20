package profile

import "testing"

func TestCommandOverrideBeatsEnv(t *testing.T) {
	withTempHome(t)
	t.Cleanup(func() { _ = SetCommandOverride("") })
	if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
		t.Fatalf("Create work: %v", err)
	}
	if err := Create("personal", CreateOpts{}); err != nil {
		t.Fatalf("Create personal: %v", err)
	}
	t.Setenv(EnvVar, "personal")
	if err := SetCommandOverride("work"); err != nil {
		t.Fatalf("SetCommandOverride: %v", err)
	}
	name, source, err := ResolveActive()
	if err != nil {
		t.Fatalf("ResolveActive: %v", err)
	}
	if name != "work" || source != SourceFlag {
		t.Errorf("ResolveActive = %q/%q, want work/%s", name, source, SourceFlag)
	}
	if err := SetCommandOverride("ghost"); err == nil {
		t.Fatalf("SetCommandOverride(ghost) should error")
	}
}

func TestCommandOverrideClear(t *testing.T) {
	withTempHome(t)
	t.Cleanup(func() { _ = SetCommandOverride("") })
	if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := Create("personal", CreateOpts{}); err != nil {
		t.Fatalf("Create personal: %v", err)
	}
	if err := SetCommandOverride("personal"); err != nil {
		t.Fatal(err)
	}
	if err := SetCommandOverride(""); err != nil {
		t.Fatal(err)
	}
	name, source, err := ResolveActive()
	if err != nil {
		t.Fatal(err)
	}
	if name != "work" || source != SourcePointer {
		t.Errorf("after clear ResolveActive = %q/%q, want work/%s", name, source, SourcePointer)
	}
}
