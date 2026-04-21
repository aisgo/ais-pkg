package repository

import (
	"database/sql"
	"testing"
)

func TestPageReadTxOptions(t *testing.T) {
	opts := pageReadTxOptions()
	if opts == nil {
		t.Fatalf("expected tx options")
	}
	if opts.Isolation != sql.LevelReadCommitted {
		t.Fatalf("expected read committed isolation, got %v", opts.Isolation)
	}
	if !opts.ReadOnly {
		t.Fatalf("expected read-only transaction")
	}
}
