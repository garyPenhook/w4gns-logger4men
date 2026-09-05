package main

import (
	"context"
	"database/sql"
)

type qsoReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *store) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if s.reader != nil {
		return s.reader.QueryContext(ctx, query, args...)
	}
	return s.db.QueryContext(ctx, query, args...)
}

// Exports hold a stable WAL read snapshot on a separate connection. Slow
// output writers do not monopolize the interactive writer connection.
func (s *store) readSnapshot(ctx context.Context) (*store, func(), error) {
	db := s.readDB
	if db == nil {
		db = s.db
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	return &store{db: s.db, reader: tx}, func() { _ = tx.Rollback() }, nil
}
