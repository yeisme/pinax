package index

import (
	"testing"

	"github.com/yeisme/pinax/internal/sqlitedsn"
)

func TestSQLiteIdentRejectsEmbeddedQuotes(t *testing.T) {
	t.Parallel()
	db, err := sqlitedsn.Open(t.TempDir() + "/quote.sqlite")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := sqliteIdent(db, `notes"evil`); err == nil {
		t.Fatal("expected embedded quote to be rejected")
	}
	got, err := sqliteIdent(db, "note_records")
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if got != "`note_records`" {
		t.Fatalf("quoted ident = %q", got)
	}
}
