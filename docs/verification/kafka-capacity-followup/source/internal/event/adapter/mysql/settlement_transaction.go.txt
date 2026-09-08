package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

// A validated duplicate must roll back any newly staged legacy settlement row.
var errRollbackNoop = errors.New("validated duplicate; rollback transaction")

// Retry only a known deadlock/lock-timeout after explicitly rolling back the
// whole transaction. A commit acknowledgement failure is ambiguous, not retried.
func (o *Outbox) settlementTransaction(ctx context.Context, action func(*sql.Tx) error) error {
	for attempt := 0; attempt < 3; attempt++ {
		err, retry := func() (error, bool) {
			tx, err := o.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
			if err != nil {
				return err, false
			}
			defer tx.Rollback()
			if err = action(tx); err != nil {
				rollbackErr := tx.Rollback()
				if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
					return errors.Join(err, rollbackErr), false
				}
				if errors.Is(err, errRollbackNoop) {
					return nil, false
				}
				var mysqlErr *driver.MySQLError
				return err, errors.As(err, &mysqlErr) && (mysqlErr.Number == 1213 || mysqlErr.Number == 1205)
			}
			return tx.Commit(), false
		}()
		if err == nil || !retry || attempt == 2 {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	panic("unreachable transaction retry")
}
