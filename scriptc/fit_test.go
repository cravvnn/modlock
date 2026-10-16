package scriptc

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/paralin/modlock/check"
	"github.com/paralin/modlock/proto/modlock/wasm"
	"github.com/paralin/modlock/sandbox"
)

// fitted returns testdata/mod.wasm fitted to the boundary. The module is
// scriptc's build of testdata/mod.ts; rebuild it from testdata with
// SCRIPTC_TARGET=wasm32-wasi scriptc build --lib --profile mod.json -o mod.wasm.
func fitted(t *testing.T) []byte {
	// Read the built module and fit it.
	t.Helper()
	module, err := os.ReadFile("testdata/mod.wasm")
	if err != nil {
		t.Fatal(err)
	}
	fit, err := Fit(module)
	if err != nil {
		t.Fatalf("fit the module: %v", err)
	}
	return fit
}

// TestProfile checks that testdata/mod.json, which built testdata/mod.wasm,
// is the profile that builds use.
func TestProfile(t *testing.T) {
	want, err := os.ReadFile("testdata/mod.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := Profile("mod.ts"); !bytes.Equal(got, want) {
		t.Fatalf("Profile(mod.ts) = %s, want testdata/mod.json:\n%s", got, want)
	}
}

// TestFitPassesCheck checks that a fitted module keeps to the host's boundary
// and starts.
func TestFitPassesCheck(t *testing.T) {
	if err := check.Module(context.Background(), fitted(t)); err != nil {
		t.Fatalf("check the fitted module: %v", err)
	}
}

// TestFitEvents delivers Start, which the mod answers after logging through
// host_call, then an event the mod throws on, whose panic must reach the log
// before the mod stops.
func TestFitEvents(t *testing.T) {
	// Load the fitted module with a host that records each log line.
	ctx := context.Background()
	var logs []string
	mod, err := sandbox.Load(ctx, fitted(t), sandbox.Options{Calls: func(call *wasm.Call) *wasm.Reply {
		request := &wasm.LogRequest{}
		if call.GetMethod() == "Log" && request.UnmarshalVT(call.GetRequest()) == nil {
			logs = append(logs, request.GetMessage())
		}
		return &wasm.Reply{}
	}})
	if err != nil {
		t.Fatalf("load the fitted module: %v", err)
	}
	t.Cleanup(func() { _ = mod.Close(ctx) })

	// Start twice: the event wrapper must answer every event, not only the
	// first.
	for range 2 {
		if err := mod.Deliver(ctx, "Start", &wasm.StartEvent{}, nil); err != nil {
			t.Fatalf("deliver Start: %v", err)
		}
	}
	if len(logs) != 2 || logs[0] != "hello from scriptc" {
		t.Fatalf("Start logged %q, want hello from scriptc twice", logs)
	}

	// An event the mod throws on logs the panic and stops the mod.
	logs = nil
	if err := mod.Deliver(ctx, "Command", &wasm.CommandEvent{}, nil); err == nil {
		t.Fatal("Command succeeded, want the mod's panic")
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "the test mod answers only Start") {
		t.Fatalf("the panic logged %q, want the thrown message", logs)
	}
	if strings.ContainsAny(logs[0], "\x01\x1f") {
		t.Fatalf("the panic log %q keeps scriptc's separators", logs[0])
	}
	if !mod.Stopped() {
		t.Fatal("the mod still runs after its panic")
	}
}
