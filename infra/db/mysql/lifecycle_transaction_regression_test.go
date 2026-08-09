package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestGlobalInitAfterCloseDoesNotReturnClosedInstance(t *testing.T) {
	closedDB := newRegressionDB(nil)
	replaceGlobalDBForTest(t, closedDB)

	if err := closedDB.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if got := GetGlobal(); got != nil {
		t.Errorf("GetGlobal() after Close() = %p, want nil", got)
	}

	reinitialized, err := Init(&Config{})
	if err == nil {
		t.Error("Init() after Close() error = nil, want empty DSN error")
	}
	if reinitialized != nil {
		t.Errorf("Init() after Close() = %p, want nil", reinitialized)
	}
	if reinitialized == closedDB {
		t.Error("Init() after Close() returned the closed global instance")
	}
}

func TestTransactionPreservesCallbackAndRollbackErrors(t *testing.T) {
	callbackErr := errors.New("callback sentinel")
	rollbackErr := errors.New("rollback sentinel")
	db := newRegressionDB(rollbackErr)
	t.Cleanup(func() { _ = db.DB.Close() })

	err := db.Transaction(context.Background(), func(*sql.Tx) error {
		return callbackErr
	})

	if !errors.Is(err, callbackErr) {
		t.Errorf("errors.Is(err, callbackErr) = false; err = %v", err)
	}
	if !errors.Is(err, rollbackErr) {
		t.Errorf("errors.Is(err, rollbackErr) = false; err = %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "transaction callback failed") || !strings.Contains(err.Error(), "rollback failed") {
		t.Errorf("Transaction() error = %q, want English callback and rollback context", err)
	}
}

func TestResetClosesAndClearsGlobal(t *testing.T) {
	db := newRegressionDB(nil)
	replaceGlobalDBForTest(t, db)

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("PingContext() before Reset() error = %v", err)
	}
	if err := Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	if got := GetGlobal(); got != nil {
		t.Errorf("GetGlobal() after Reset() = %p, want nil", got)
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Error("PingContext() after Reset() error = nil, want closed database error")
	}
	if err := Reset(); err != nil {
		t.Errorf("second Reset() error = %v", err)
	}
}

func TestStaleCloseDoesNotClearReplacementGlobal(t *testing.T) {
	staleDB := newRegressionDB(nil)
	replacementDB := newRegressionDB(nil)
	replaceGlobalDBForTest(t, staleDB)
	t.Cleanup(func() { _ = replacementDB.DB.Close() })

	globalMu.Lock()
	globalDB = replacementDB
	globalMu.Unlock()

	if err := staleDB.Close(); err != nil {
		t.Fatalf("stale Close() error = %v", err)
	}
	if got := GetGlobal(); got != replacementDB {
		t.Errorf("GetGlobal() after stale Close() = %p, want replacement %p", got, replacementDB)
	}
}

func TestGlobalLifecycleConcurrent(t *testing.T) {
	firstDB := newRegressionDB(nil)
	replaceGlobalDBForTest(t, firstDB)

	for i := 0; i < 200; i++ {
		candidate := firstDB
		if i > 0 {
			candidate = newRegressionDB(nil)
			globalMu.Lock()
			globalDB = candidate
			globalMu.Unlock()
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		var initializedDB *DB
		var initErr, closeErr, resetErr error
		wg.Add(4)

		go func() {
			defer wg.Done()
			<-start
			initializedDB, initErr = Init(&Config{})
		}()
		go func(db *DB) {
			defer wg.Done()
			<-start
			closeErr = db.Close()
		}(candidate)
		go func() {
			defer wg.Done()
			<-start
			resetErr = Reset()
		}()
		go func() {
			defer wg.Done()
			<-start
			_ = GetGlobal()
		}()

		close(start)
		wg.Wait()

		if closeErr != nil {
			t.Fatalf("iteration %d: Close() error = %v", i, closeErr)
		}
		if resetErr != nil {
			t.Fatalf("iteration %d: Reset() error = %v", i, resetErr)
		}
		if (initializedDB == nil) != (initErr != nil) {
			t.Fatalf("iteration %d: Init() returned db=%p, err=%v", i, initializedDB, initErr)
		}
		if got := GetGlobal(); got != nil {
			t.Fatalf("iteration %d: GetGlobal() = %p, want nil", i, got)
		}
	}
}

func replaceGlobalDBForTest(t *testing.T, db *DB) {
	t.Helper()

	globalMu.Lock()
	previous := globalDB
	globalDB = db
	globalMu.Unlock()

	t.Cleanup(func() {
		globalMu.Lock()
		globalDB = previous
		globalMu.Unlock()
		_ = db.DB.Close()
	})
}

func newRegressionDB(rollbackErr error) *DB {
	return &DB{
		DB:     sql.OpenDB(regressionConnector{rollbackErr: rollbackErr}),
		config: &Config{},
	}
}

type regressionConnector struct {
	rollbackErr error
}

func (c regressionConnector) Connect(context.Context) (driver.Conn, error) {
	return &regressionConn{rollbackErr: c.rollbackErr}, nil
}

func (regressionConnector) Driver() driver.Driver {
	return regressionDriver{}
}

type regressionDriver struct{}

func (regressionDriver) Open(string) (driver.Conn, error) {
	return &regressionConn{}, nil
}

type regressionConn struct {
	rollbackErr error
}

func (*regressionConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}

func (*regressionConn) Close() error {
	return nil
}

func (c *regressionConn) Begin() (driver.Tx, error) {
	return &regressionTx{rollbackErr: c.rollbackErr}, nil
}

func (c *regressionConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}

type regressionTx struct {
	rollbackErr error
}

func (*regressionTx) Commit() error {
	return nil
}

func (tx *regressionTx) Rollback() error {
	return tx.rollbackErr
}
