package sqlx

import (
	"context"
	"database/sql"
)

type commonSqlConn struct {
	onError func(context.Context, error)
	accept  func(error) bool
}
type sessionConn interface {
	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (db *commonSqlConn) queryRows(ctx context.Context, scanner func(*sql.Rows) error,
	q string, args ...any) (err error) {
	ctx, span := startSpan(ctx, "QueryRow")
	defer func() {
		endSpan(span, err)
	}()

	return query(ctx, nil, scanner, q, args...)
}

func query(ctx context.Context, conn sessionConn, scanner func(*sql.Rows) error,
	q string, args ...any) error {

	rows, _ := conn.QueryContext(ctx, q, args...)
	return scanner(rows)
}
