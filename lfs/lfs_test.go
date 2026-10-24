package lfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/paralin/modlock/space"
	"github.com/sirupsen/logrus"
)

// TestAgentKeepsObjectsAcrossDaemons uploads an object through the agent,
// stops the daemon's Space, and downloads the object from a new one.
func TestAgentKeepsObjectsAcrossDaemons(t *testing.T) {
	// Write an object larger than one block for git-lfs to upload.
	ctx := t.Context()
	dir := t.TempDir()
	data := bytes.Repeat([]byte("vpk "), 300_000)
	sum := sha256.Sum256(data)
	oid := hex.EncodeToString(sum[:])
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Upload it through a daemon, then stop that daemon.
	db := filepath.Join(dir, "space.s4db")
	size := strconv.Itoa(len(data))
	upload := `{"event":"upload","oid":"` + oid + `","size":` + size + `,"path":` + strconv.Quote(src) + `}`
	if out := runAgent(ctx, t, db, dir, "upload", upload); !strings.Contains(out, `"complete"`) || strings.Contains(out, `"error"`) {
		t.Fatalf("upload answered %s", out)
	}

	// A new daemon on the same database serves it back.
	download := `{"event":"download","oid":"` + oid + `","size":` + size + `}`
	out := runAgent(ctx, t, db, dir, "download", download)
	_, after, ok := strings.Cut(out, `"path":"`)
	path, _, _ := strings.Cut(after, `"`)
	got, err := os.ReadFile(path)
	if !ok || err != nil || !bytes.Equal(got, data) {
		t.Fatalf("download answered %s: %v", out, err)
	}
}

// runAgent opens the Space at db as a daemon does, runs one agent transfer
// of operation with request against it, and returns the agent's answer.
func runAgent(ctx context.Context, t *testing.T, db, tmpDir, operation, request string) string {
	// Open the Space.
	le := logrus.NewEntry(logrus.New())
	sp, err := space.Open(ctx, le, db)
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Release()

	// Serve it on the daemon's services.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := NewServer()
	mux := srpc.NewMux()
	if err := server.Register(mux); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(ctx, le, sp) }()

	// Run the agent as git-lfs would, over an in-memory daemon connection.
	init := `{"event":"init","operation":"` + operation + `","remote":"origin","concurrent":false,"concurrenttransfers":1}`
	in := strings.NewReader(init + "\n" + request + "\n" + `{"event":"terminate"}` + "\n")
	var out bytes.Buffer
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	if err := RunAgent(ctx, client, tmpDir, in, &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
