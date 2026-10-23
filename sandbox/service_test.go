package sandbox

import (
	"testing"

	"github.com/paralin/modlock/src/wasm"
	"github.com/paralin/modlock/src/wasm/manifest"
)

// TestCallService refuses a service call, as a host without the service does,
// so a mod takes its no-service path instead of decoding an empty reply.
func TestCallService(t *testing.T) {
	request, _ := (&wasm.ServiceCall{Service: "lobby", Method: "setup"}).MarshalVT()
	reply := (&Sandbox{}).answer(&manifest.Manifest{Slug: "mod"}, &wasm.Call{Method: "CallService", Request: request})
	if want := "this host provides no service lobby"; reply.GetError() != want {
		t.Fatalf("error %q, want %q", reply.GetError(), want)
	}
}
