package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/native"
	"github.com/egoist/mygo/plugins/sqlite/internal/sqlitecore"
)

// ErrClosed is returned when an operation uses a closed database.
var ErrClosed = errors.New("sqlite: database is closed")

// Error is an SQLite error. Code is the primary result code; ExtendedCode
// distinguishes such errors as unique, foreign-key and not-null constraints.
type Error struct {
	Code         int
	ExtendedCode int
	Message      string
}

func (e *Error) Error() string { return fmt.Sprintf("sqlite: %s (code %d)", e.Message, e.ExtendedCode) }

// OpenOptions configure a connection. Foreign keys are enabled by default.
type OpenOptions struct {
	// ReadOnly opens an existing database without allowing writes.
	ReadOnly bool
	// BusyTimeout is how long an operation waits for another connection's
	// lock: five seconds when zero, no wait when negative. Cancellation
	// stops the wait early.
	BusyTimeout time.Duration
}

// Result describes a statement. Changes counts directly affected rows;
// LastInsertID is SQLite's last inserted rowid on this connection.
type Result struct {
	Changes      int64 `json:"changes"`
	LastInsertID int64 `json:"lastInsertId"`
}

// Rows holds columns and their values in order, preserving duplicate column
// names. Values are nil, int64, float64, string or []byte. BLOBs are copied
// before the next step, so they remain valid after the statement closes.
type Rows struct {
	Columns []string `json:"columns"`
	Values  [][]any  `json:"rows"`
}

// Statement is one parameterized statement in a transaction.
type Statement struct {
	SQL  string
	Args []any
}

// DB is a connection to the plugin's SQLite library. It is safe from any
// goroutine; operations serialize on the connection and may be canceled
// while waiting for it. Use Transaction for atomic groups of statements.
type DB struct {
	gate   chan struct{}
	life   context.Context
	cancel context.CancelFunc
	conn   sqlitecore.Conn // guarded by gate; nil once closed
}

var engine = sync.OnceValues(sqlitecore.NewEngine)

// Load loads the plugin's SQLite, a native module, once. Open calls it
// lazily, so binding the plugin and generating clients need no library.
func Load() error {
	_, err := engine()
	return err
}

// Open opens path (or ":memory:") with SQLite, which the plugin compiles
// with Zig into a native module, without cgo. It creates a missing file
// unless ReadOnly. The parent directory must already exist. Close the
// returned connection when done.
func Open(ctx context.Context, path string, opts OpenOptions) (*DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" || strings.IndexByte(path, 0) >= 0 || !utf8.ValidString(path) {
		return nil, errors.New("sqlite: database path must be nonempty UTF-8 without NUL")
	}
	e, err := engine()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := opts.BusyTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ms := timeout.Milliseconds()
	if timeout > 0 && ms == 0 {
		ms = 1
	}
	ms = max(0, min(ms, math.MaxInt32))
	conn, err := e.Open(path, opts.ReadOnly, int(ms))
	if err != nil {
		return nil, sqliteError(err)
	}
	life, cancel := context.WithCancel(context.Background())
	db := &DB{conn: conn, gate: make(chan struct{}, 1), life: life, cancel: cancel}
	db.gate <- struct{}{}
	if err := ctx.Err(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Close cancels in-flight operations, waits for their statements to be
// finalized, and closes the connection. Repeated calls are harmless.
func (d *DB) Close() error {
	d.cancel()
	<-d.gate
	defer func() { d.gate <- struct{}{} }()
	if d.conn == nil {
		return nil
	}
	if err := d.conn.Close(); err != nil {
		return sqliteError(err)
	}
	d.conn = nil
	return nil
}

// Execute runs exactly one statement with positional parameters. Supported
// values are nil, bool, integers, finite floats, strings, []byte and
// json.Number. SQL text may not contain NUL. Transaction control, ATTACH
// and VACUUM INTO are rejected; use Transaction for groups of statements.
func (d *DB) Execute(ctx context.Context, sql string, args ...any) (result Result, err error) {
	err = d.operation(ctx, func(func()) error {
		var e error
		result, _, e = d.run(sql, args, false)
		return e
	})
	return
}

// Query runs exactly one statement and copies its rows. It also supports
// statements with RETURNING. For large results, select a bounded page.
func (d *DB) Query(ctx context.Context, sql string, args ...any) (rows Rows, err error) {
	err = d.operation(ctx, func(func()) error {
		var e error
		_, rows, e = d.run(sql, args, true)
		return e
	})
	return
}

// Transaction runs statements under BEGIN IMMEDIATE, without interleaving
// other operations on this connection. Any error, including cancellation,
// rolls back the batch. Results are returned only after COMMIT succeeds.
func (d *DB) Transaction(ctx context.Context, statements []Statement) (results []Result, err error) {
	err = d.operation(ctx, func(detach func()) (err error) {
		if len(statements) == 0 {
			results = []Result{}
			return nil
		}
		if err = d.control("BEGIN IMMEDIATE"); err != nil {
			return err
		}
		committed := false
		defer func() {
			if !committed {
				// Cleanup must run even when the operation is canceled.
				detach()
				if rollback := d.control("ROLLBACK"); rollback != nil {
					// SQLITE_INTERRUPT may already have rolled the transaction back.
					if !d.conn.Autocommit() {
						err = errors.Join(err, rollback)
					}
				}
			}
		}()
		results = make([]Result, 0, len(statements))
		for i, s := range statements {
			if err = ctx.Err(); err != nil {
				return err
			}
			var result Result
			result, _, err = d.run(s.SQL, s.Args, false)
			if err != nil {
				return fmt.Errorf("sqlite: statement %d: %w", i+1, err)
			}
			results = append(results, result)
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if d.life.Err() != nil {
			return ErrClosed
		}
		if err = d.control("COMMIT"); err != nil {
			return err
		}
		committed = true
		return nil
	})
	if err != nil {
		results = nil
	}
	return
}

// operation runs fn alone on the connection. Canceling ctx, or closing
// the connection, interrupts the statement running; fn's detach stops
// that, waiting for an interruption that started, as before cleanup.
func (d *DB) operation(ctx context.Context, fn func(detach func()) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.life.Done():
		return ErrClosed
	case <-d.gate:
	}
	defer func() { d.gate <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.conn == nil || d.life.Err() != nil {
		return ErrClosed
	}
	conn := d.conn
	watch := func(c context.Context) func() {
		end, done := make(chan struct{}), make(chan struct{})
		stop := context.AfterFunc(c, func() {
			defer close(done)
			// SQLite ignores an interruption while no statement runs, as
			// before the first step of one being prepared: again until
			// the operation ends.
			for {
				conn.Interrupt()
				select {
				case <-end:
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
		})
		return func() {
			// An interruption that started ends before the next statement.
			close(end)
			if !stop() {
				<-done
			}
		}
	}
	stopCaller, stopClose := watch(ctx), watch(d.life)
	detached := false
	detach := func() {
		if !detached {
			detached = true
			stopCaller()
			stopClose()
		}
	}
	defer detach()
	err := fn(detach)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.life.Err() != nil {
			return ErrClosed
		}
	}
	return err
}

// sqliteError makes an Error of an error of SQLite's that the module
// reports, by its extended code.
func sqliteError(err error) error {
	var e *native.Error
	if errors.As(err, &e) && e.Domain == "SQLite" {
		code := int(e.Number)
		return &Error{Code: code & 255, ExtendedCode: code, Message: e.Message}
	}
	if errors.As(err, &e) && e.Domain == "" && e.Code == "" {
		return errors.New(e.Message)
	}
	return err
}

func checkSQL(sql string) error {
	if len(sql) > math.MaxInt32-1 || strings.IndexByte(sql, 0) >= 0 || !utf8.ValidString(sql) {
		return errors.New("sqlite: SQL must be UTF-8 without NUL and fit in a C int")
	}
	return nil
}

func (d *DB) run(sql string, args []any, query bool) (Result, Rows, error) {
	if err := checkSQL(sql); err != nil {
		return Result{}, Rows{}, err
	}
	values := make([]sqlitecore.Value, len(args))
	for i, arg := range args {
		v, err := value(arg)
		if err != nil {
			return Result{}, Rows{}, fmt.Errorf("sqlite: parameter %d: %w", i+1, err)
		}
		values[i] = v
	}
	out, err := d.conn.Run(sql, values, query)
	if err != nil {
		return Result{}, Rows{}, sqliteError(err)
	}
	rows := Rows{Columns: []string{}, Values: [][]any{}}
	if query {
		rows.Columns = append(rows.Columns, out.Columns...)
		if n := len(out.Columns); n > 0 {
			for i := 0; i+n <= len(out.Values); i += n {
				row := make([]any, n)
				for j, v := range out.Values[i : i+n] {
					row[j] = goValue(v)
				}
				rows.Values = append(rows.Values, row)
			}
		}
	}
	return Result{Changes: out.Changes, LastInsertID: out.LastInsertID}, rows, nil
}

func (d *DB) control(sql string) error {
	return sqliteError(d.conn.Control(sql))
}

// value converts a parameter of Go's to SQLite's.
func value(arg any) (sqlitecore.Value, error) {
	switch v := arg.(type) {
	case nil:
		return sqlitecore.Value{Type: sqlitecore.Null}, nil
	case []byte:
		if len(v) > math.MaxInt32 {
			return sqlitecore.Value{}, errors.New("BLOB is too large")
		}
		return sqlitecore.Value{Type: sqlitecore.Blob, Blob: v}, nil
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return value(i)
		}
		if !strings.ContainsAny(string(v), ".eE") {
			return sqlitecore.Value{}, errors.New("integer exceeds SQLite's signed 64-bit range")
		}
		f, err := v.Float64()
		if err != nil {
			return sqlitecore.Value{}, err
		}
		return value(f)
	}
	v := reflect.ValueOf(arg)
	switch v.Kind() {
	case reflect.Bool:
		n := int64(0)
		if v.Bool() {
			n = 1
		}
		return sqlitecore.Value{Type: sqlitecore.Integer, Int: n}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return sqlitecore.Value{Type: sqlitecore.Integer, Int: v.Int()}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Uint() > math.MaxInt64 {
			return sqlitecore.Value{}, errors.New("integer exceeds SQLite's signed 64-bit range")
		}
		return sqlitecore.Value{Type: sqlitecore.Integer, Int: int64(v.Uint())}, nil
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return sqlitecore.Value{}, errors.New("number must be finite")
		}
		return sqlitecore.Value{Type: sqlitecore.Real, Real: f}, nil
	case reflect.String:
		s := v.String()
		if len(s) > math.MaxInt32 || !utf8.ValidString(s) {
			return sqlitecore.Value{}, errors.New("text must be UTF-8 and fit in a C int")
		}
		return sqlitecore.Value{Type: sqlitecore.Text, Text: s}, nil
	}
	return sqlitecore.Value{}, fmt.Errorf("unsupported value %T", arg)
}

// goValue converts a column of SQLite's to Go's: nil, int64, float64,
// string or []byte.
func goValue(v sqlitecore.Value) any {
	switch v.Type {
	case sqlitecore.Integer:
		return v.Int
	case sqlitecore.Real:
		return v.Real
	case sqlitecore.Text:
		return v.Text
	case sqlitecore.Blob:
		if v.Blob == nil {
			return []byte{}
		}
		return v.Blob
	}
	return nil
}
