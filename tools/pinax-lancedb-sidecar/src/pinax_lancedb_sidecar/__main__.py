from __future__ import annotations

import json
import os
import sys
from contextlib import contextmanager
from pathlib import Path
from typing import Any

import lancedb

from pinax_lancedb_sidecar import SCHEMA_VERSION

TABLE_NAME = "chunks"


class SidecarError(Exception):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code
        self.message = message


def main() -> int:
    op = sys.argv[1] if len(sys.argv) > 1 else ""
    try:
        request = json.load(sys.stdin)
        if request.get("schema_version") != SCHEMA_VERSION:
            raise SidecarError("schema_version_invalid", "unsupported sidecar schema version")
        if op == "doctor":
            response = doctor(request)
        elif op == "rebuild":
            response = rebuild(request)
        elif op == "search":
            response = search(request)
        else:
            raise SidecarError("operation_invalid", "unknown sidecar operation")
        print(json.dumps(response, ensure_ascii=False, separators=(",", ":")))
        return 0
    except SidecarError as exc:
        print(json.dumps(error_response(exc.code, exc.message), ensure_ascii=False, separators=(",", ":")))
        return 2
    except Exception as exc:  # noqa: BLE001 - sidecar must return protocol JSON on all failures.
        print(json.dumps(error_response("sidecar_internal_error", str(exc)), ensure_ascii=False, separators=(",", ":")))
        return 1


def doctor(request: dict[str, Any]) -> dict[str, Any]:
    store = store_path(request)
    store.mkdir(parents=True, exist_ok=True)
    with silence_native_stderr():
        db = lancedb.connect(str(store))
        tables = table_names(db)
    return success({"backend": "lancedb", "dependency": "lancedb", "tables": tables})


def rebuild(request: dict[str, Any]) -> dict[str, Any]:
    store = store_path(request)
    store.mkdir(parents=True, exist_ok=True)
    chunks = [normalize_chunk(chunk) for chunk in request.get("chunks", [])]
    with silence_native_stderr():
        db = lancedb.connect(str(store))
        if chunks:
            db.create_table(TABLE_NAME, data=chunks, mode="overwrite")
        elif TABLE_NAME in table_names(db):
            db.drop_table(TABLE_NAME)
    return success({"backend": "lancedb", "documents": int(request.get("documents") or 0), "chunks": len(chunks)})


def search(request: dict[str, Any]) -> dict[str, Any]:
    store = store_path(request)
    vector = request.get("query_vector") or []
    if not vector:
        raise SidecarError("query_vector_required", "query vector is required")
    limit = int(request.get("limit") or 8)
    with silence_native_stderr():
        db = lancedb.connect(str(store))
        if TABLE_NAME not in table_names(db):
            raise SidecarError("kb_index_missing", "KB semantic index is missing")
        table = db.open_table(TABLE_NAME)
        rows = table.search(vector, vector_column_name="vector").limit(limit).to_list()
    hits = [row_to_hit(row) for row in rows]
    return success({"backend": "lancedb", "total": len(hits), "hits": hits})


def store_path(request: dict[str, Any]) -> Path:
    raw = str(request.get("store_uri") or "").strip()
    if not raw:
        raise SidecarError("store_uri_required", "store_uri is required")
    return Path(raw)


def table_names(db: Any) -> list[str]:
    if hasattr(db, "list_tables"):
        response = db.list_tables()
        if hasattr(response, "tables"):
            return list(response.tables)
        return list(response)
    return list(db.table_names())


@contextmanager
def silence_native_stderr():
    saved = os.dup(2)
    try:
        with open(os.devnull, "wb") as devnull:
            os.dup2(devnull.fileno(), 2)
            yield
    finally:
        os.dup2(saved, 2)
        os.close(saved)


def normalize_chunk(chunk: dict[str, Any]) -> dict[str, Any]:
    if "chunk_text" in chunk:
        raise SidecarError("chunk_text_forbidden", "sidecar chunks must not include full chunk text")
    vector = [float(value) for value in chunk.get("vector") or []]
    if not vector:
        raise SidecarError("vector_required", "chunk vector is required")
    row = dict(chunk)
    row["vector"] = vector
    row["tags"] = list(row.get("tags") or [])
    return row


def row_to_hit(row: dict[str, Any]) -> dict[str, Any]:
    distance = float(row.get("_distance") or 0.0)
    score = 1.0 / (1.0 + max(distance, 0.0))
    return {
        "chunk_id": row.get("chunk_id", ""),
        "note_id": row.get("note_id", ""),
        "path": row.get("vault_path", ""),
        "title": row.get("title", ""),
        "heading_path": row.get("heading_path", ""),
        "preview": row.get("preview", ""),
        "score": score,
        "provider": row.get("provider", ""),
        "model": row.get("embedding_model", ""),
        "tags": row.get("tags") or [],
        "kind": row.get("kind", ""),
        "status": row.get("status", ""),
    }


def success(extra: dict[str, Any]) -> dict[str, Any]:
    return {"schema_version": SCHEMA_VERSION, "status": "success", **extra}


def error_response(code: str, message: str) -> dict[str, Any]:
    return {"schema_version": SCHEMA_VERSION, "status": "failed", "error": {"code": code, "message": message}}


if __name__ == "__main__":
    raise SystemExit(main())
