package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fakeDrivebridge installs a python fake of the DriveBridge CLI onto PATH.
// It speaks the machine envelope (spec_version 1.0) and implements the
// storage adopt contract, space list, ls/stat/download used by Pinax.
type fakeDrivebridge struct {
	statePath string
	logPath   string
}

const fakeDrivebridgeScript = `#!/usr/bin/env python3
import json, os, shutil, sys

args = sys.argv[1:]
if args and args[0] == "--json":
    args = args[1:]
state_path = os.environ.get("FAKE_DRIVEBRIDGE_STATE", "")
log_path = os.environ.get("FAKE_DRIVEBRIDGE_LOG", "")
if log_path:
    with open(log_path, "a") as fh:
        fh.write(" ".join(args) + "\n")
state = {}
if state_path and os.path.exists(state_path):
    with open(state_path) as fh:
        state = json.load(fh)

def emit(command, summary, data, status="success", code="", message=""):
    out = {"spec_version": "1.0", "mode": "json", "command": command, "status": status,
           "summary": summary if status == "success" else message}
    if data is not None:
        out["data"] = data
    if status != "success":
        out["error"] = {"code": code or "fake_error", "message": message}
    print(json.dumps(out))
    sys.exit(0 if status == "success" else 1)

def flags(argv):
    out, i = {}, 0
    while i < len(argv):
        if argv[i].startswith("--"):
            if i + 1 < len(argv) and not argv[i + 1].startswith("--"):
                out[argv[i][2:]] = argv[i + 1]
                i += 2
                continue
            out[argv[i][2:]] = "true"
        i += 1
    return out

fail = state.get("fail", {})
key2 = " ".join(args[:2]) if len(args) >= 2 else " ".join(args)
if key2 in fail:
    spec = fail[key2]
    emit(key2.replace(" ", "."), None, None, status="failed",
         code=spec.get("code", "fake_error"), message=spec.get("message", "fake failure"))

if args[:2] == ["storage", "adopt"]:
    f = flags(args[2:])
    if len(args) > 2 and args[2] == "status":
        adopts = state.get("adopts", [])
        emit("storage.adopt.status", "%d adopt record(s)." % len(adopts),
             {"count": len(adopts), "adopts": adopts})
    kind, space = f.get("kind", ""), f.get("space", "")
    loc = f.get("root", "") or f.get("remote-path", "")
    emit("storage.adopt",
         "Adopted %s %s as space %s for consumer %s; copied=false." % (kind, loc, space, f.get("consumer", "")),
         {"consumer": f.get("consumer", ""), "kind": kind, "space": space, "location": loc, "copied": False})
if args[:2] == ["storage", "unadopt"]:
    emit("storage.unadopt", None, None, status="failed", code="unknown_command",
         message='unknown command "unadopt" for "drivebridge storage"')
if args[:2] == ["space", "list"]:
    spaces = state.get("spaces", [])
    emit("space.list", "%d configured space(s)." % len(spaces), {"count": len(spaces), "spaces": spaces})
if args[:1] == ["ls"]:
    f = flags(args[1:])
    space = f.get("space", "")
    rows = []
    for x in state.get("files", []):
        if x.get("space") != space:
            continue
        rows.append({"ref": x["ref"], "space_id": space, "name": x["name"], "dir": x.get("dir", ""),
                     "version": x["version"], "sha256": x.get("sha256", ""), "size_bytes": x.get("size", 0),
                     "is_directory": bool(x.get("dir_only", False))})
    emit("ls", "%d file(s)." % len(rows), {"count": len(rows), "files": rows})
if args[:1] == ["stat"]:
    f = flags(args[1:])
    ref = f.get("ref", "")
    x = next((y for y in state.get("files", []) if y["ref"] == ref), None)
    if x is None:
        emit("stat", None, None, status="failed", code="not_found", message="reference not found: " + ref)
    cur = dict(x)
    bump = state.get("stat_bump", {}).get(ref)
    if bump:
        cur.update(bump)
    emit("stat", "%s %d bytes version=%s" % (cur["name"], cur.get("size", 0), cur["version"]),
         {"ref": ref, "space_id": cur.get("space", ""), "version": cur["version"], "name": cur["name"],
          "size_bytes": cur.get("size", 0), "sha256": cur.get("sha256", "")})
if args[:1] == ["download"]:
    f = flags(args[1:])
    ref, out_path = f.get("ref", ""), f.get("out", "")
    x = next((y for y in state.get("files", []) if y["ref"] == ref), None)
    if x is None:
        emit("download", None, None, status="failed", code="not_found", message="reference not found: " + ref)
    if ref in state.get("partial_download", {}):
        os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
        with open(x["path"], "rb") as src, open(out_path, "wb") as dst:
            data = src.read()
            dst.write(data[:len(data)//2])
        emit("download", None, None, status="failed", code="interrupted",
             message="fake interrupted transfer for " + ref)
    os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
    shutil.copyfile(x["path"], out_path)
    emit("download", "Downloaded via op-fake.", {"id": "op-fake", "status": "completed", "file_ref": ref})
emit(" ".join(args), None, None, status="failed", code="unknown_command",
     message="fake drivebridge does not implement: " + " ".join(args))
`

func newFakeDrivebridge(t *testing.T) *fakeDrivebridge {
	t.Helper()
	binDir := t.TempDir()
	statePath := filepath.Join(binDir, "state.json")
	logPath := filepath.Join(binDir, "calls.log")
	if err := os.WriteFile(filepath.Join(binDir, "drivebridge"), []byte(fakeDrivebridgeScript), 0o755); err != nil {
		t.Fatalf("write fake drivebridge: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write fake state: %v", err)
	}
	t.Setenv("FAKE_DRIVEBRIDGE_STATE", statePath)
	t.Setenv("FAKE_DRIVEBRIDGE_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &fakeDrivebridge{statePath: statePath, logPath: logPath}
}

func (f *fakeDrivebridge) setState(t *testing.T, state map[string]any) {
	t.Helper()
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal fake state: %v", err)
	}
	if err := os.WriteFile(f.statePath, b, 0o644); err != nil {
		t.Fatalf("write fake state: %v", err)
	}
}

func (f *fakeDrivebridge) calls(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(f.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read fake log: %v", err)
	}
	return string(b)
}

// fakeDrivebridgeFile describes one remote file the fake serves; path is the
// local source of the bytes.
type fakeDrivebridgeFile struct {
	Space   string `json:"space"`
	Ref     string `json:"ref"`
	Name    string `json:"name"`
	Dir     string `json:"dir"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	Path    string `json:"path"`
}

func fakeFiles(files ...fakeDrivebridgeFile) []map[string]any {
	out := make([]map[string]any, 0, len(files))
	for _, f := range files {
		b, _ := json.Marshal(f)
		m := map[string]any{}
		_ = json.Unmarshal(b, &m)
		out = append(out, m)
	}
	return out
}
