from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from pinax_lancedb_sidecar import SCHEMA_VERSION


class SidecarTest(unittest.TestCase):
    def run_sidecar(self, op: str, payload: dict) -> tuple[int, dict]:
        proc = subprocess.run(
            [sys.executable, "-m", "pinax_lancedb_sidecar", op],
            input=json.dumps(payload),
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        self.assertEqual(proc.stderr, "")
        return proc.returncode, json.loads(proc.stdout)

    def test_rebuild_and_search_real_lancedb(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            store = str(Path(tmp) / "lancedb")
            base = {"schema_version": SCHEMA_VERSION, "store_uri": store, "backend": "lancedb"}
            code, doctor = self.run_sidecar("doctor", base)
            self.assertEqual(code, 0)
            self.assertEqual(doctor["status"], "success")

            chunk = {
                "chunk_id": "chunk_alpha",
                "note_id": "note_alpha",
                "vault_path": "notes/alpha.md",
                "title": "Alpha",
                "heading_path": "Intro",
                "preview": "bounded semantic preview",
                "content_hash": "content_hash",
                "chunk_hash": "chunk_hash",
                "token_count": 3,
                "tags": ["kb"],
                "kind": "reference",
                "status": "active",
                "embedding_model": "fake-hash-v1",
                "embedding_dim": 3,
                "provider": "fake",
                "backend": "lancedb",
                "vector": [1.0, 0.0, 0.0],
                "indexed_at": "2026-06-19T00:00:00Z",
            }
            code, rebuild = self.run_sidecar("rebuild", {**base, "documents": 1, "chunks": [chunk]})
            self.assertEqual(code, 0)
            self.assertEqual(rebuild["chunks"], 1)

            code, search = self.run_sidecar("search", {**base, "query_vector": [1.0, 0.0, 0.0], "limit": 1})
            self.assertEqual(code, 0)
            self.assertEqual(search["status"], "success")
            self.assertEqual(search["hits"][0]["path"], "notes/alpha.md")
            self.assertNotIn("chunk_text", json.dumps(search))

    def test_rejects_full_chunk_text(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            payload = {
                "schema_version": SCHEMA_VERSION,
                "store_uri": str(Path(tmp) / "lancedb"),
                "backend": "lancedb",
                "chunks": [{"chunk_id": "chunk_bad", "chunk_text": "full body", "vector": [1.0]}],
            }
            code, response = self.run_sidecar("rebuild", payload)
            self.assertNotEqual(code, 0)
            self.assertEqual(response["error"]["code"], "chunk_text_forbidden")


if __name__ == "__main__":
    unittest.main()
