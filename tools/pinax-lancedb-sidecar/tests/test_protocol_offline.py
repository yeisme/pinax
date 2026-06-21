from __future__ import annotations

import importlib
import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest import mock


class FakeTable:
    def __init__(self, rows: list[dict]) -> None:
        self.rows = rows
        self._limit = 8

    def search(self, vector: list[float], vector_column_name: str = "vector") -> "FakeTable":
        if vector_column_name != "vector":
            raise AssertionError(f"unexpected vector column: {vector_column_name}")
        return self

    def limit(self, limit: int) -> "FakeTable":
        self._limit = limit
        return self

    def to_list(self) -> list[dict]:
        return [dict(row, _distance=float(index)) for index, row in enumerate(self.rows[: self._limit])]


class FakeLanceDBConnection:
    stores: dict[str, dict[str, list[dict]]] = {}

    def __init__(self, uri: str) -> None:
        self.tables = self.stores.setdefault(uri, {})

    def list_tables(self) -> list[str]:
        return sorted(self.tables)

    def create_table(self, name: str, data: list[dict], mode: str = "overwrite") -> None:
        if mode != "overwrite":
            raise AssertionError(f"unexpected mode: {mode}")
        self.tables[name] = list(data)

    def drop_table(self, name: str) -> None:
        self.tables.pop(name, None)

    def open_table(self, name: str) -> FakeTable:
        return FakeTable(self.tables[name])


def fake_lancedb_module() -> types.SimpleNamespace:
    def connect(uri: str) -> FakeLanceDBConnection:
        return FakeLanceDBConnection(uri)

    return types.SimpleNamespace(connect=connect)


def load_sidecar_with_fake_lancedb():
    FakeLanceDBConnection.stores = {}
    sys.modules.pop("pinax_lancedb_sidecar.__main__", None)
    with mock.patch.dict(sys.modules, {"lancedb": fake_lancedb_module()}):
        return importlib.import_module("pinax_lancedb_sidecar.__main__")


class OfflineProtocolTest(unittest.TestCase):
    def test_doctor_rebuild_and_search_without_python_package_install(self) -> None:
        sidecar = load_sidecar_with_fake_lancedb()
        with tempfile.TemporaryDirectory() as tmp:
            store = str(Path(tmp) / "lancedb")
            base = {"schema_version": sidecar.SCHEMA_VERSION, "store_uri": store, "backend": "lancedb"}

            doctor = sidecar.doctor(base)
            self.assertEqual(doctor["status"], "success")
            self.assertEqual(doctor["tables"], [])

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
            rebuild = sidecar.rebuild({**base, "documents": 1, "chunks": [chunk]})
            self.assertEqual(rebuild["chunks"], 1)

            search = sidecar.search({**base, "query_vector": [1.0, 0.0, 0.0], "limit": 1})
            self.assertEqual(search["status"], "success")
            self.assertEqual(search["hits"][0]["path"], "notes/alpha.md")
            self.assertNotIn("chunk_text", str(search))

    def test_rebuild_rejects_full_chunk_text_without_real_lancedb(self) -> None:
        sidecar = load_sidecar_with_fake_lancedb()
        with tempfile.TemporaryDirectory() as tmp:
            payload = {
                "schema_version": sidecar.SCHEMA_VERSION,
                "store_uri": str(Path(tmp) / "lancedb"),
                "backend": "lancedb",
                "chunks": [{"chunk_id": "chunk_bad", "chunk_text": "full body", "vector": [1.0]}],
            }
            with self.assertRaises(sidecar.SidecarError) as ctx:
                sidecar.rebuild(payload)
            self.assertEqual(ctx.exception.code, "chunk_text_forbidden")


if __name__ == "__main__":
    unittest.main()
