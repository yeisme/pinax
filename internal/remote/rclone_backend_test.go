package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRcloneBackendCatCopytoLsfAndMissing(t *testing.T) {
	root := installFakeRclone(t)
	ctx := context.Background()
	store, err := NewStore(ctx, "rclone://onedrive/PinaxSync")
	if err != nil {
		t.Fatalf("NewStore rclone: %v", err)
	}

	rev, err := store.Put(ctx, "workspaces/personal/head.json", []byte(`{"revision_id":"rev_a"}`), "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if rev == "" {
		t.Fatalf("Put returned empty revision")
	}
	data, gotRev, err := store.Get(ctx, "workspaces/personal/head.json")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != `{"revision_id":"rev_a"}` || gotRev == "" {
		t.Fatalf("Get data/rev = %q %q", data, gotRev)
	}
	if _, err := os.Stat(filepath.Join(root, "onedrive", "PinaxSync", "workspaces", "personal", "head.json")); err != nil {
		t.Fatalf("copyto did not write expected object: %v", err)
	}
	statRev, err := store.Stat(ctx, "workspaces/personal/head.json")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if statRev == "" {
		t.Fatalf("Stat rev is empty")
	}
	listed, err := store.(ExtendedBlobStore).List(ctx, "workspaces/personal")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].Key != "workspaces/personal/head.json" || listed[0].Revision == "" {
		t.Fatalf("List = %#v", listed)
	}
	if _, _, err := store.Get(ctx, "missing.json"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("missing Get err = %v", err)
	}
	if _, err := store.Stat(ctx, "missing.json"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("missing Stat err = %v", err)
	}
}

func TestRcloneBackendCommandFailureTimeoutAndRedaction(t *testing.T) {
	installFakeRclone(t)
	ctx := context.Background()
	store, err := NewStore(ctx, "rclone://onedrive/PinaxSync")
	if err != nil {
		t.Fatalf("NewStore rclone: %v", err)
	}

	t.Setenv("FAKE_RCLONE_FAIL_COPYTO", "Authorization: Bearer raw-token token=raw path=notes/alpha.md refresh_token=raw-refresh client_secret=raw-secret")
	_, err = store.Put(ctx, "head.json", []byte("body"), "")
	if err == nil {
		t.Fatalf("copyto failure returned nil")
	}
	msg := err.Error()
	for _, leaked := range []string{"raw-token", "raw-refresh", "raw-secret", "notes/alpha.md", "Authorization: Bearer raw-token"} {
		if strings.Contains(msg, leaked) {
			t.Fatalf("rclone error leaked %q in %q", leaked, msg)
		}
	}
	for _, want := range []string{"[REDACTED]", "[REDACTED_PATH]", "[REDACTED_SECRET]"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("rclone error missing redaction %q in %q", want, msg)
		}
	}

	t.Setenv("FAKE_RCLONE_FAIL_COPYTO", "")
	t.Setenv("FAKE_RCLONE_SLEEP", "2s")
	shortCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = store.Put(shortCtx, "slow.json", []byte("body"), "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout err = %v", err)
	}
}

func TestRcloneBackendRejectsUnsafeKeysAndDoesNotClaimConditionalWrites(t *testing.T) {
	installFakeRclone(t)
	store, err := NewStore(context.Background(), "rclone://onedrive/PinaxSync")
	if err != nil {
		t.Fatalf("NewStore rclone: %v", err)
	}
	conditional, ok := store.(ConditionalWriteCapability)
	if !ok || conditional.SupportsConditionalWrites() {
		t.Fatalf("rclone store must declare conditional writes unsupported")
	}
	if _, err := store.Put(context.Background(), "../outside.json", []byte("x"), ""); err == nil {
		t.Fatalf("unsafe Put key accepted")
	}
	if _, _, err := store.Get(context.Background(), "/tmp/outside.json"); err == nil {
		t.Fatalf("unsafe Get key accepted")
	}
}

func installFakeRclone(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	binDir := t.TempDir()
	script := `#!/usr/bin/env python3
import hashlib, os, pathlib, shutil, sys, time
root = pathlib.Path(os.environ["FAKE_RCLONE_ROOT"])
sleep = os.environ.get("FAKE_RCLONE_SLEEP", "")
if sleep:
    time.sleep(float(sleep.rstrip("s")))

def local_path(target):
    if ":" not in target:
        print("bad target", file=sys.stderr)
        sys.exit(7)
    remote, path = target.split(":", 1)
    path = path.strip("/")
    return root / remote / path

def rel_key(base, child):
    return child.relative_to(base).as_posix()

args = sys.argv[1:]
if not args:
    print("missing command", file=sys.stderr)
    sys.exit(2)
cmd = args[0]
if cmd == "cat":
    p = local_path(args[1])
    if not p.exists():
        print("object not found path=notes/alpha.md Authorization: Bearer raw-token", file=sys.stderr)
        sys.exit(3)
    sys.stdout.buffer.write(p.read_bytes())
elif cmd == "copyto":
    fail = os.environ.get("FAKE_RCLONE_FAIL_COPYTO", "")
    if fail:
        print(fail, file=sys.stderr)
        sys.exit(9)
    src = pathlib.Path(args[1])
    dst = local_path(args[2])
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(src, dst)
elif cmd == "lsf":
    target = args[-1]
    base = local_path(target)
    if base.is_file():
        files = [base]
        parent = base.parent
    else:
        parent = base
        files = sorted([p for p in base.rglob("*") if p.is_file()]) if base.exists() else []
    for p in files:
        data = p.read_bytes()
        stamp = "2026-06-12T00:00:00Z"
        print(f"{len(data)};{rel_key(parent, p)};{stamp}")
elif cmd == "deletefile":
    p = local_path(args[1])
    try:
        p.unlink()
    except FileNotFoundError:
        pass
else:
    print("unsupported fake rclone command " + cmd, file=sys.stderr)
    sys.exit(2)
`
	path := filepath.Join(binDir, "rclone")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake rclone: %v", err)
	}
	t.Setenv("FAKE_RCLONE_ROOT", root)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return root
}
