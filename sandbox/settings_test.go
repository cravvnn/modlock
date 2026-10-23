package sandbox

import (
	"path/filepath"
	"testing"

	"github.com/paralin/modlock/settings"
	"github.com/paralin/modlock/src/host_app/control"
	"github.com/paralin/modlock/src/wasm"
	manifestpb "github.com/paralin/modlock/src/wasm/manifest"
	settingspb "github.com/paralin/modlock/src/wasm/settings"
)

// TestSettings answers a mod's settings calls: the player starts at the
// default, keeps an accepted value across sessions, and a refused value
// leaves the kept one.
func TestSettings(t *testing.T) {
	// Declare one choice and keep it in a file across sessions.
	manifest := &manifestpb.Manifest{Slug: "hud", Settings: []*settingspb.Setting{{
		Key:     "layout",
		Label:   "Layout",
		Kind:    settingspb.Setting_KIND_CHOICE,
		Choices: []*settingspb.SettingChoice{{Value: "corner"}, {Value: "center"}},
	}}}
	path := filepath.Join(t.TempDir(), "settings.json")
	session := func() *Sandbox {
		file, err := settings.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		return &Sandbox{settings: file, config: Config{Events: func(*control.HostEvent) {}}}
	}
	read := func(s *Sandbox) string {
		request, _ := (&settingspb.PlayerSettingRequest{Key: "layout"}).MarshalVT()
		response := &settingspb.SettingResponse{}
		if err := response.UnmarshalVT(s.answer(manifest, &wasm.Call{Method: "PlayerSetting", Request: request}).GetResponse()); err != nil {
			t.Fatal(err)
		}
		return response.GetValue()
	}
	set := func(s *Sandbox, value string) {
		request, _ := (&settingspb.SetPlayerSettingRequest{Key: "layout", Value: value}).MarshalVT()
		s.answer(manifest, &wasm.Call{Method: "SetPlayerSetting", Request: request})
	}

	// Start at the default, keep the accepted value and refuse the other.
	first := session()
	if got := read(first); got != "corner" {
		t.Fatalf("started at %q, want corner", got)
	}
	set(first, "center")
	set(first, "top")
	if got := read(session()); got != "center" {
		t.Fatalf("the next session read %q, want center", got)
	}
}
