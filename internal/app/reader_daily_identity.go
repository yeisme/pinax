package app

import (
	"context"
	"encoding/json"

	"github.com/yeisme/pinax/internal/operation"
)

// Creation receipts establish a calendar date -> object identity relation;
// paths/titles alone cannot recover an arbitrarily relocated old journal.
// The returned ID is still resolved through current reader visibility.
func readerDailyRecordedIdentity(ctx context.Context, root, date string) (string, error) {
	store, cleanup, err := operation.OpenReadSnapshot(root)
	if operation.IsCode(err, operation.CodeNotFound) {
		return "", nil
	}
	if err != nil {
		return "", ReaderError("reader_unavailable", "Daily identity history is unavailable", "")
	}
	defer cleanup()
	id, after := "", ""
	for count := 0; ; {
		rows, err := store.ListCapability(ctx, writerDailyApplyCapability, after, 200)
		if err != nil {
			return "", ReaderError("reader_unavailable", "Daily identity history could not be read", "")
		}
		count += len(rows)
		if count > 1000 {
			return "", ReaderError("reader_unsupported", "Daily identity history exceeds its lookup budget", "")
		}
		for _, row := range rows {
			if row.Status == operation.StatusFailed {
				continue
			}
			var result dailyOperationResult
			if json.Unmarshal(row.ResultJSON, &result) != nil || result.Date == "" {
				return "", ReaderError("reader_unavailable", "An earlier daily operation needs owner recovery", "")
			}
			if result.Date != date {
				continue
			}
			if row.Status != operation.StatusSucceeded || result.NoteID == "" || result.RecordEventID == "" || result.ContentDigest == "" || row.RevisionAfter != result.ContentDigest {
				return "", ReaderError("reader_unavailable", "The daily identity lacks a completed owner receipt", "")
			}
			if id != "" && id != result.NoteID {
				return "", ReaderError("reader_unsupported", "Multiple daily identities match this date", "")
			}
			id = result.NoteID
		}
		if len(rows) < 200 {
			return id, nil
		}
		after = rows[len(rows)-1].OperationID
	}
}
