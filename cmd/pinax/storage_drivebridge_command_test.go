package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256SumFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// fakeDrivebridgeScript is a compact fake of the DriveBridge CLI machine
// surface used by the attach/doctor remote-mode contract tests.
const cmdFakeDrivebridgeScript = `#!/usr/bin/env python3
import json, os, sys

args = sys.argv[1:]
if args and args[0] == "--json":
    args = args[1:]
state = {}
with open(os.environ["FAKE_DRIVEBRIDGE_STATE"]) as fh:
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
    rows = []
    for x in state.get("files", []):
        if x.get("space") != f.get("space", ""):
            continue
        rows.append({"ref": x["ref"], "space_id": x["space"], "name": x["name"], "dir": x.get("dir", ""),
                     "version": x["version"], "sha256": x.get("sha256", ""), "size_bytes": x.get("size", 0),
                     "is_directory": False})
    emit("ls", "%d file(s)." % len(rows), {"count": len(rows), "files": rows})
if args[:1] == ["stat"]:
    f = flags(args[1:])
    x = next((y for y in state.get("files", []) if y["ref"] == f.get("ref", "")), None)
    if x is None:
        emit("stat", None, None, status="failed", code="not_found", message="reference not found")
    emit("stat", "%s bytes" % x["name"],
         {"ref": x["ref"], "space_id": x["space"], "version": x["version"], "name": x["name"],
          "size_bytes": x.get("size", 0), "sha256": x.get("sha256", "")})
if args[:1] == ["download"]:
    import shutil
    f = flags(args[1:])
    x = next((y for y in state.get("files", []) if y["ref"] == f.get("ref", "")), None)
    if x is None:
        emit("download", None, None, status="failed", code="not_found", message="reference not found")
    os.makedirs(os.path.dirname(f["out"]) or ".", exist_ok=True)
    shutil.copyfile(x["path"], f["out"])
    emit("download", "Downloaded via op-fake.", {"id": "op-fake", "status": "completed", "file_ref": f["ref"]})
emit(" ".join(args), None, None, status="failed", code="unknown_command",
     message="fake drivebridge does not implement: " + " ".join(args))
`

func installCmdFakeDrivebridge(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "drivebridge"), []byte(cmdFakeDrivebridgeScript), 0o755); err != nil {
		t.Fatalf("write fake drivebridge: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write fake state: %v", err)
	}
	t.Setenv("FAKE_DRIVEBRIDGE_STATE", filepath.Join(binDir, "state.json"))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Remote API mode must treat the storage group as a local control plane (like
// capsa/sync): attach/doctor run locally instead of failing with
// remote_command_unsupported.
func TestStorageAttachDrivebridgeRemoteModeLocal(t *testing.T) {
	installCmdFakeDrivebridge(t)
	t.Setenv("NO_COLOR", "1")
	// vault bootstrap stays outside remote mode for the setup phase.
	t.Setenv("PINAX_API_URL", "")
	vault := t.TempDir()
	runCLI(t, "init", vault, "--title", "Vault", "--json")
	runCLI(t, "storage", "set", "local", "--root", vault, "--vault", vault, "--json")
	t.Setenv("PINAX_API_URL", "http://127.0.0.1:1")

	attach, stderr, err := runCLISeparate("storage", "attach-drivebridge", "--space", "pinax-vault", "--vault", vault, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("attach in remote mode: err=%v stderr=%q stdout=%s", err, stderr, attach)
	}
	if strings.Contains(attach, "remote_command_unsupported") {
		t.Fatalf("storage attach must stay local under PINAX_API_URL: %s", attach)
	}
	var envelope struct {
		Facts map[string]string `json:"facts"`
	}
	if err := json.Unmarshal([]byte(attach), &envelope); err != nil {
		t.Fatalf("attach output not a json envelope: %v\n%s", err, attach)
	}
	if envelope.Facts["drivebridge_attached"] != "true" || envelope.Facts["copied"] != "false" {
		t.Fatalf("attach facts = %#v", envelope.Facts)
	}
	if _, err := os.Stat(filepath.Join(vault, ".pinax", "drivebridge-attach.yaml")); err != nil {
		t.Fatalf("attach record missing: %v", err)
	}

	doctor, stderr, err := runCLISeparate("storage", "doctor", "--vault", vault, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("doctor in remote mode: err=%v stderr=%q stdout=%s", err, stderr, doctor)
	}
	if !strings.Contains(doctor, "provider-plaintext") && !strings.Contains(doctor, "adopt_local") {
		t.Fatalf("doctor facts missing drivebridge content mode: %s", doctor)
	}
	if strings.Contains(doctor, "remote_command_unsupported") {
		t.Fatalf("storage doctor must stay local under PINAX_API_URL: %s", doctor)
	}
}

func TestStorageDrivebridgeNotInstalledContract(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	vault := t.TempDir()
	runCLI(t, "init", vault, "--title", "Vault", "--json")
	runCLI(t, "storage", "set", "s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax/", "--vault", vault, "--json")

	out, stderr, err := runCLISeparate("storage", "attach-drivebridge", "--space", "pinax-vault", "--vault", vault, "--json")
	if err == nil || !strings.Contains(out+stderr, "drivebridge_not_installed") {
		t.Fatalf("expected drivebridge_not_installed, err=%v out=%s stderr=%q", err, out, stderr)
	}
	// 未 attach 时 status/doctor 与 owner 配置命令不受缺二进制影响。
	out, stderr, err = runCLISeparate("storage", "doctor", "--vault", vault, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("doctor without drivebridge: err=%v stderr=%q", err, stderr)
	}
	if !strings.Contains(out, "\"drivebridge_attached\": \"false\"") && !strings.Contains(out, "\"drivebridge_attached\":\"false\"") {
		t.Fatalf("doctor facts missing drivebridge_attached=false: %s", out)
	}
	runCLI(t, "backend", "add", "local", "local-1", "--root", vault, "--vault", vault, "--json")
}

// Machine modes must stay free of credentials and note bodies: hydrate lands
// note bytes on disk, but stdout may never echo them.
func TestStorageDrivebridgeMachineModesRedaction(t *testing.T) {
	installCmdFakeDrivebridge(t)
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("NO_COLOR", "1")
	vault := t.TempDir()
	runCLI(t, "init", vault, "--title", "Vault", "--json")
	runCLI(t, "storage", "set", "s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax/", "--vault", vault, "--json")
	runCLI(t, "storage", "attach-drivebridge", "--space", "pinax-vault", "--vault", vault, "--json")

	source := t.TempDir()
	notePath := filepath.Join(source, "secret-note.md")
	noteBody := "---\nschema_version: pinax.note.v1\ntitle: T\n---\n\nNOTE_BODY_MARKER_DO_NOT_LEAK\n"
	if err := os.WriteFile(notePath, []byte(noteBody), 0o644); err != nil {
		t.Fatalf("write source note: %v", err)
	}
	sum := sha256SumFile(t, notePath)
	state := map[string]any{
		"files": []map[string]any{{
			"space": "pinax-vault", "ref": "pinax-vault:notes/secret-note.md", "name": "secret-note.md",
			"dir": "notes", "version": "v1", "sha256": sum, "size": len(noteBody), "path": notePath,
		}},
	}
	statePath := os.Getenv("FAKE_DRIVEBRIDGE_STATE")
	b, _ := json.Marshal(state)
	if err := os.WriteFile(statePath, b, 0o644); err != nil {
		t.Fatalf("write fake state: %v", err)
	}

	for _, mode := range []string{"--json", "--agent", "--events"} {
		out, stderr, err := runCLISeparate("storage", "hydrate", "--space", "pinax-vault", "--vault", vault, mode)
		if err != nil || stderr != "" {
			t.Fatalf("hydrate %s: err=%v stderr=%q", mode, err, stderr)
		}
		for _, forbidden := range []string{"NOTE_BODY_MARKER_DO_NOT_LEAK", "AKIA", "Authorization: Bearer"} {
			if strings.Contains(out, forbidden) {
				t.Fatalf("hydrate %s leaked %q: %s", mode, forbidden, out)
			}
		}
		if _, err := os.Stat(filepath.Join(vault, "notes", "secret-note.md")); err != nil {
			t.Fatalf("hydrate %s did not land the note: %v", mode, err)
		}
	}
}
