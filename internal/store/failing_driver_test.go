package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"sync/atomic"

	// modernc.org/sqlite registers the pure-Go "sqlite" driver, which the
	// failing wrapper below delegates to.
	_ "modernc.org/sqlite"
)

// afInjectedErr is the sentinel error returned by the failing driver when a
// failure is armed. It stands in for an arbitrary Data_Store operation
// failure so Property 20 can verify rollback atomicity.
var afInjectedErr = errors.New("af: injected data store failure")

// afDriverName is the unique name under which the failing driver is registered
// with database/sql.
const afDriverName = "af-failing-sqlite"

var (
	afRegisterOnce sync.Once
	// afFlags maps a DSN to the failure flag controlling connections opened
	// for that DSN. Each test iteration uses a unique temp-file DSN, so each
	// gets its own independent flag while a single driver stays registered.
	afFlags sync.Map // map[string]*afFailFlag
)

// afFailFlag controls injected failures for connections opened against a
// particular DSN. It is safe for concurrent use because database/sql may open
// several pooled connections.
type afFailFlag struct {
	failOp     atomic.Bool // force write/read statements to fail when set
	failCommit atomic.Bool // force transaction Commit to fail when set
}

func (f *afFailFlag) armOp()      { f.failOp.Store(true) }
func (f *afFailFlag) armCommit()  { f.failCommit.Store(true) }
func (f *afFailFlag) disarm()     { f.failOp.Store(false); f.failCommit.Store(false) }
func (f *afFailFlag) opArmed() bool     { return f.failOp.Load() }
func (f *afFailFlag) commitArmed() bool { return f.failCommit.Load() }

// afEnsureDriver registers the failing driver exactly once.
func afEnsureDriver() {
	afRegisterOnce.Do(func() {
		sql.Register(afDriverName, &afDriver{base: afBaseDriver()})
	})
}

// afBaseDriver returns the underlying modernc.org/sqlite driver instance by
// briefly opening (and closing) a database with the real driver.
func afBaseDriver() driver.Driver {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic("af: open base sqlite driver: " + err.Error())
	}
	d := db.Driver()
	_ = db.Close()
	return d
}

// afNewFlag registers and returns a fresh failure flag for the given DSN.
func afNewFlag(dsn string) *afFailFlag {
	f := &afFailFlag{}
	afFlags.Store(dsn, f)
	return f
}

// afFlagFor returns the flag registered for a DSN, or a disarmed default.
func afFlagFor(dsn string) *afFailFlag {
	if v, ok := afFlags.Load(dsn); ok {
		return v.(*afFailFlag)
	}
	return &afFailFlag{}
}

// afDriver wraps the real sqlite driver so opened connections can be forced to
// fail on demand.
type afDriver struct {
	base driver.Driver
}

func (d *afDriver) Open(name string) (driver.Conn, error) {
	c, err := d.base.Open(name)
	if err != nil {
		return nil, err
	}
	return &afConn{base: c, flag: afFlagFor(name)}, nil
}

// afConn wraps a driver.Conn, delegating to the base connection but injecting
// failures when the associated flag is armed.
type afConn struct {
	base driver.Conn
	flag *afFailFlag
}

var (
	_ driver.Conn               = (*afConn)(nil)
	_ driver.ConnPrepareContext = (*afConn)(nil)
	_ driver.ConnBeginTx        = (*afConn)(nil)
	_ driver.ExecerContext      = (*afConn)(nil)
	_ driver.QueryerContext     = (*afConn)(nil)
)

func (c *afConn) Prepare(query string) (driver.Stmt, error) {
	s, err := c.base.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &afStmt{base: s, flag: c.flag}, nil
}

func (c *afConn) Close() error { return c.base.Close() }

func (c *afConn) Begin() (driver.Tx, error) {
	//nolint:staticcheck // Begin is required by the driver.Conn interface.
	tx, err := c.base.Begin()
	if err != nil {
		return nil, err
	}
	return &afTx{base: tx, flag: c.flag}, nil
}

func (c *afConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if cb, ok := c.base.(driver.ConnBeginTx); ok {
		tx, err := cb.BeginTx(ctx, opts)
		if err != nil {
			return nil, err
		}
		return &afTx{base: tx, flag: c.flag}, nil
	}
	return c.Begin()
}

func (c *afConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if cp, ok := c.base.(driver.ConnPrepareContext); ok {
		s, err := cp.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		return &afStmt{base: s, flag: c.flag}, nil
	}
	return c.Prepare(query)
}

func (c *afConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.flag.opArmed() {
		return nil, afInjectedErr
	}
	if e, ok := c.base.(driver.ExecerContext); ok {
		return e.ExecContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *afConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.flag.opArmed() {
		return nil, afInjectedErr
	}
	if q, ok := c.base.(driver.QueryerContext); ok {
		return q.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

// afTx wraps a driver.Tx so Commit can be forced to fail. On a forced commit
// failure the underlying transaction is rolled back so nothing is persisted,
// mirroring a real commit-time Data_Store failure.
type afTx struct {
	base driver.Tx
	flag *afFailFlag
}

func (t *afTx) Commit() error {
	if t.flag.commitArmed() {
		_ = t.base.Rollback()
		return afInjectedErr
	}
	return t.base.Commit()
}

func (t *afTx) Rollback() error { return t.base.Rollback() }

// afStmt wraps a driver.Stmt, injecting failures on execution when armed. It
// is used for the prepared-statement fallback path.
type afStmt struct {
	base driver.Stmt
	flag *afFailFlag
}

var (
	_ driver.Stmt             = (*afStmt)(nil)
	_ driver.StmtExecContext  = (*afStmt)(nil)
	_ driver.StmtQueryContext = (*afStmt)(nil)
)

func (s *afStmt) Close() error  { return s.base.Close() }
func (s *afStmt) NumInput() int { return s.base.NumInput() }

func (s *afStmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.flag.opArmed() {
		return nil, afInjectedErr
	}
	//nolint:staticcheck // Exec is required by the driver.Stmt interface.
	return s.base.Exec(args)
}

func (s *afStmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.flag.opArmed() {
		return nil, afInjectedErr
	}
	//nolint:staticcheck // Query is required by the driver.Stmt interface.
	return s.base.Query(args)
}

func (s *afStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if s.flag.opArmed() {
		return nil, afInjectedErr
	}
	if e, ok := s.base.(driver.StmtExecContext); ok {
		return e.ExecContext(ctx, args)
	}
	vals, err := afNamedToValue(args)
	if err != nil {
		return nil, err
	}
	//nolint:staticcheck // fallback for drivers without StmtExecContext.
	return s.base.Exec(vals)
}

func (s *afStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if s.flag.opArmed() {
		return nil, afInjectedErr
	}
	if q, ok := s.base.(driver.StmtQueryContext); ok {
		return q.QueryContext(ctx, args)
	}
	vals, err := afNamedToValue(args)
	if err != nil {
		return nil, err
	}
	//nolint:staticcheck // fallback for drivers without StmtQueryContext.
	return s.base.Query(vals)
}

// afNamedToValue converts named driver values to positional values for the
// deprecated Stmt.Exec/Query fallback path.
func afNamedToValue(named []driver.NamedValue) ([]driver.Value, error) {
	vals := make([]driver.Value, len(named))
	for i, nv := range named {
		vals[i] = nv.Value
	}
	return vals, nil
}
