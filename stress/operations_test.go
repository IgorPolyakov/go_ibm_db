package stress_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	ibmdb "github.com/ibmdb/go_ibm_db"
	"github.com/ibmdb/go_ibm_db/api"
)

const operationCount = 16

var nextTable atomic.Uint64

type liveSession struct {
	conn   *sql.Conn
	insert *sql.Stmt
	table  string
}

func tableSQL(pattern, table string) string {
	return strings.ReplaceAll(pattern, "{table}", table)
}

func openSession(ctx context.Context, db *sql.DB, cfg liveConfig) (*liveSession, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	session := &liveSession{
		conn:  conn,
		table: fmt.Sprintf("gibs_%d_%d", os.Getpid(), nextTable.Add(1)),
	}
	if _, err = conn.ExecContext(ctx, tableSQL(cfg.createTable, session.table)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	session.insert, err = conn.PrepareContext(ctx, tableSQL(cfg.insertRow, session.table))
	if err != nil {
		session.Close(cfg)
		return nil, err
	}
	return session, nil
}

func (s *liveSession) Close(cfg liveConfig) {
	if s.insert != nil {
		_ = s.insert.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.operationTimeout)
	defer cancel()
	_, _ = s.conn.ExecContext(ctx, tableSQL(cfg.dropTable, s.table))
	_ = s.conn.Close()
}

func (s *liveSession) clear(ctx context.Context, cfg liveConfig) error {
	_, err := s.conn.ExecContext(ctx, tableSQL(cfg.deleteRows, s.table))
	return err
}

func runOperation(ctx context.Context, db *sql.DB, pool *ibmdb.Pool, session *liveSession, cfg liveConfig, operation byte) error {
	var value int64
	switch operation % operationCount {
	case 0: // direct query
		return db.QueryRowContext(ctx, cfg.query).Scan(&value)
	case 1: // rows, metadata, array fetch, early close
		rows, err := db.QueryContext(ctx, cfg.rowsQuery)
		if err != nil {
			return err
		}
		columns, err := rows.ColumnTypes()
		if err != nil {
			_ = rows.Close()
			return err
		}
		if len(columns) != 1 {
			_ = rows.Close()
			return fmt.Errorf("rows query returned %d columns, want 1", len(columns))
		}
		count := 0
		for rows.Next() && count < 32 {
			if err = rows.Scan(&value); err != nil {
				_ = rows.Close()
				return err
			}
			count++
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if count == 0 {
			return errors.New("rows query returned no rows")
		}
		return nil
	case 2: // prepared read
		stmt, err := db.PrepareContext(ctx, cfg.query)
		if err != nil {
			return err
		}
		err = stmt.QueryRowContext(ctx).Scan(&value)
		closeErr := stmt.Close()
		if err != nil {
			return err
		}
		return closeErr
	case 3: // pinned connection
		return session.conn.QueryRowContext(ctx, cfg.query).Scan(&value)
	case 4: // connection health
		return db.PingContext(ctx)
	case 5: // prepared scalar bind, result, parameterized read
		result, err := session.insert.ExecContext(ctx, int64(7))
		if err != nil {
			return err
		}
		if _, err = result.RowsAffected(); err != nil {
			return err
		}
		_, _ = result.LastInsertId() // unsupported on some servers
		if err = session.conn.QueryRowContext(ctx, tableSQL(cfg.selectRow, session.table), int64(7)).Scan(&value); err != nil {
			return err
		}
		return session.clear(ctx, cfg)
	case 6: // array bind followed by scalar bind on the same statement
		if _, err := session.insert.ExecContext(ctx, []int64{1, 2}); err != nil {
			return err
		}
		if _, err := session.insert.ExecContext(ctx, int64(3)); err != nil {
			return err
		}
		return session.clear(ctx, cfg)
	case 7: // direct exec
		result, err := session.conn.ExecContext(ctx, tableSQL(cfg.deleteRows, session.table))
		if err != nil {
			return err
		}
		_, err = result.RowsAffected()
		return err
	case 8: // transaction commit
		tx, err := session.conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, cfg.query).Scan(&value); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	case 9: // transaction rollback
		tx, err := session.conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, cfg.query).Scan(&value); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Rollback()
	case 10: // SQLGetInfo through the driver connection
		return session.conn.Raw(func(raw any) error {
			conn, ok := raw.(*ibmdb.Conn)
			if !ok {
				return fmt.Errorf("unexpected driver connection %T", raw)
			}
			name, err := conn.GetInfo(api.SQL_DBMS_NAME)
			if err != nil {
				return err
			}
			if name == "" {
				return errors.New("empty SQL_DBMS_NAME")
			}
			return nil
		})
	case 11: // custom pool acquire, use, release
		pooled := pool.Open(cfg.dsn)
		if pooled == nil {
			return errors.New("custom pool returned no connection")
		}
		err := pooled.PingContext(ctx)
		pooled.Close()
		return err
	case 12: // canceled context
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		err := db.QueryRowContext(canceled, cfg.query).Scan(&value)
		if !errors.Is(err, context.Canceled) {
			return fmt.Errorf("canceled query returned %v", err)
		}
		return nil
	case 13: // legacy driver.Query, Prepare, Stmt.Exec, Rows.Next and NextResultSet
		err := session.conn.Raw(func(raw any) error {
			conn, ok := raw.(*ibmdb.Conn)
			if !ok {
				return fmt.Errorf("unexpected driver connection %T", raw)
			}
			rows, err := conn.Query(cfg.query, nil)
			if err != nil {
				return err
			}
			values := make([]driver.Value, 1)
			if err = rows.Next(values); err != nil && !errors.Is(err, io.EOF) {
				_ = rows.Close()
				return err
			}
			if next, ok := rows.(driver.RowsNextResultSet); ok {
				if err = next.NextResultSet(); err != nil && !errors.Is(err, io.EOF) {
					_ = rows.Close()
					return err
				}
			}
			if err = rows.Close(); err != nil {
				return err
			}
			stmt, err := conn.Prepare(tableSQL(cfg.insertRow, session.table))
			if err != nil {
				return err
			}
			_, err = stmt.Exec([]driver.Value{int64(11)}) //nolint:staticcheck // SA1019: intentionally covers the legacy interface
			closeErr := stmt.Close()
			if err != nil {
				return err
			}
			return closeErr
		})
		if err != nil {
			return err
		}
		return session.clear(ctx, cfg)
	case 14: // prepared query with a bound parameter
		if _, err := session.insert.ExecContext(ctx, int64(17)); err != nil {
			return err
		}
		stmt, err := session.conn.PrepareContext(ctx, tableSQL(cfg.selectRow, session.table))
		if err != nil {
			return err
		}
		err = stmt.QueryRowContext(ctx, int64(17)).Scan(&value)
		closeErr := stmt.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		return session.clear(ctx, cfg)
	default: // legacy driver.Begin and Tx.Rollback
		return session.conn.Raw(func(raw any) error {
			conn, ok := raw.(*ibmdb.Conn)
			if !ok {
				return fmt.Errorf("unexpected driver connection %T", raw)
			}
			tx, err := conn.Begin()
			if err != nil {
				return err
			}
			return tx.Rollback()
		})
	}
}
