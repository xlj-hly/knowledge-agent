package knowledge

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

type sqlVectorCall struct {
	query string
	args  []any
}

func assertScopedSQL(t *testing.T, calls []sqlVectorCall, scope []string) {
	t.Helper()
	writes := 0
	for _, call := range calls {
		for _, value := range scope {
			if strings.Contains(call.query, value) {
				t.Fatal("身份值被拼接进SQL文本")
			}
		}
		if !strings.HasPrefix(call.query, "DELETE FROM") && !strings.HasPrefix(call.query, "INSERT INTO") {
			continue
		}
		writes++
		for _, value := range scope {
			found := false
			for _, arg := range call.args {
				if arg == value {
					found = true
				}
			}
			if !found {
				t.Fatalf("写入或清理遗漏范围参数: %s", call.query)
			}
		}
	}
	if writes == 0 {
		t.Fatal("没有执行实际索引写入")
	}
}
func sqlVectorStore(provider string) domain.VectorStore {
	config := map[string]string{"host": "localhost", "port": "5432", "database": "agent", "username": "tester", "password": "private-sql-fixture", "schema": "public", "sslmode": "disable", "table": "agent_knowledge"}
	if provider == "doris" {
		config["port"] = "9030"
		delete(config, "schema")
		delete(config, "sslmode")
	}
	return domain.VectorStore{Name: "索引连接", Provider: provider, Config: config}
}

type vectorTestPG struct {
	calls                            []sqlVectorCall
	closed, rollbackBound, committed bool
	queryRows                        [][]any
	batchError                       error
}

func (c *vectorTestPG) record(query string, args []any) {
	c.calls = append(c.calls, sqlVectorCall{query, append([]any(nil), args...)})
}
func (c *vectorTestPG) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	c.record(q, args)
	return pgconn.NewCommandTag("OK"), nil
}
func (c *vectorTestPG) Query(_ context.Context, q string, args ...any) (pgx.Rows, error) {
	c.record(q, args)
	values := c.queryRows
	if strings.Contains(q, "pg_catalog.pg_tables") {
		values = [][]any{{"agent_knowledge_3"}, {"agent_knowledge_bad"}, {"agent_knowledge_03"}}
	}
	return &vectorTestPGRows{values: values}, nil
}
func (c *vectorTestPG) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	c.record(q, args)
	name := "agent_knowledge_3"
	if strings.Contains(q, "to_regclass") {
		return vectorTestPGRow{values: []any{&name}}
	}
	return vectorTestPGRow{values: []any{"0.8.0"}}
}
func (c *vectorTestPG) Begin(context.Context) (pgx.Tx, error) { return &vectorTestPGTx{conn: c}, nil }
func (c *vectorTestPG) Close(ctx context.Context) error {
	_, bounded := ctx.Deadline()
	c.closed = bounded
	return nil
}

type vectorTestPGRow struct{ values []any }

func (r vectorTestPGRow) Scan(dest ...any) error {
	for i, target := range dest {
		reflect.ValueOf(target).Elem().Set(reflect.ValueOf(r.values[i]))
	}
	return nil
}

type vectorTestPGRows struct {
	pgx.Rows
	values [][]any
	index  int
}

func (r *vectorTestPGRows) Close()     {}
func (r *vectorTestPGRows) Err() error { return nil }
func (r *vectorTestPGRows) Next() bool { r.index++; return r.index <= len(r.values) }
func (r *vectorTestPGRows) Scan(dest ...any) error {
	return (vectorTestPGRow{values: r.values[r.index-1]}).Scan(dest...)
}

type vectorTestPGTx struct {
	pgx.Tx
	conn *vectorTestPG
}

func (tx *vectorTestPGTx) Exec(ctx context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	return tx.conn.Exec(ctx, q, args...)
}
func (tx *vectorTestPGTx) SendBatch(_ context.Context, batch *pgx.Batch) pgx.BatchResults {
	for _, q := range batch.QueuedQueries {
		tx.conn.record(q.SQL, q.Arguments)
	}
	return vectorTestPGBatch{err: tx.conn.batchError}
}
func (tx *vectorTestPGTx) Commit(context.Context) error { tx.conn.committed = true; return nil }
func (tx *vectorTestPGTx) Rollback(ctx context.Context) error {
	_, tx.conn.rollbackBound = ctx.Deadline()
	return nil
}

type vectorTestPGBatch struct {
	pgx.BatchResults
	err error
}

func (b vectorTestPGBatch) Close() error { return b.err }

func TestPGVectorParameterizedScopeAndFailureCleanup(t *testing.T) {
	scope := []string{"tenant' OR TRUE--", "base'--", "doc'--", "attempt'--"}
	for _, fail := range []bool{false, true} {
		conn := &vectorTestPG{}
		if fail {
			conn.batchError = errors.New("server echoed private-sql-fixture")
		}
		adapter := &pgvectorDriver{dial: func(_ context.Context, cfg *pgx.ConnConfig) (pgvectorConnection, error) {
			if cfg.Password != "private-sql-fixture" {
				t.Fatal("密码没有按当前连接传入")
			}
			return conn, nil
		}}
		err := adapter.ReplaceDocument(context.Background(), sqlVectorStore("pgvector"), scope[0], scope[1], scope[2], scope[3], []port.VectorPoint{{ID: "71", Vector: []float32{1, 0, 0}}})
		if (err != nil) != fail {
			t.Fatalf("失败状态不符: %v", err)
		}
		if err != nil && strings.Contains(err.Error(), "private-sql-fixture") {
			t.Fatal("SQL错误泄漏了上游敏感原文")
		}
		if !conn.closed || !conn.rollbackBound || conn.committed == fail {
			t.Fatal("连接、事务或回滚生命周期错误")
		}
		assertScopedSQL(t, conn.calls, scope)
		if !strings.Contains(conn.calls[0].query, `"public"."agent_knowledge_3"`) {
			t.Fatal("没有按维度创建隔离表")
		}
	}
}
func TestPGVectorSearchRejectsForeignScopeAndDeletePreservesAttempt(t *testing.T) {
	conn := &vectorTestPG{queryRows: [][]any{{"71", "tenant", "base", "doc", "attempt", 0.9}}}
	adapter := &pgvectorDriver{dial: func(context.Context, *pgx.ConnConfig) (pgvectorConnection, error) { return conn, nil }}
	got, err := adapter.Search(context.Background(), sqlVectorStore("pgvector"), "tenant", "base", []float32{1, 0, 0}, 3)
	if err != nil || len(got) != 1 || got[0].ID != "71" {
		t.Fatalf("有效命中丢失: %+v %v", got, err)
	}
	conn.queryRows[0][1] = "foreign"
	if _, err := adapter.Search(context.Background(), sqlVectorStore("pgvector"), "tenant", "base", []float32{1, 0, 0}, 3); err == nil {
		t.Fatal("返回了跨企业命中")
	}
	conn.calls = nil
	if err := adapter.DeleteAttempt(context.Background(), sqlVectorStore("pgvector"), "tenant", "base", "doc", "old-attempt"); err != nil {
		t.Fatal(err)
	}
	deletes := 0
	for _, call := range conn.calls {
		if strings.HasPrefix(call.query, "DELETE FROM") {
			deletes++
			if !strings.Contains(call.query, "attempt=$4") || !reflect.DeepEqual(call.args, []any{"tenant", "base", "doc", "old-attempt"}) {
				t.Fatal("处理代删除扩展了范围")
			}
		}
	}
	if deletes != 1 {
		t.Fatal("枚举表没有排除不规范维度后缀")
	}
}

type vectorTestConnector struct{ conn *vectorTestMySQL }

func (c vectorTestConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (c vectorTestConnector) Driver() driver.Driver                        { return vectorTestDriver{c.conn} }

type vectorTestDriver struct{ conn *vectorTestMySQL }

func (d vectorTestDriver) Open(string) (driver.Conn, error) { return d.conn, nil }

type vectorTestMySQL struct {
	calls      []sqlVectorCall
	closed     bool
	searchRows [][]driver.Value
	queryError error
	rowError   error
}

func (c *vectorTestMySQL) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *vectorTestMySQL) Close() error { c.closed = true; return nil }
func (c *vectorTestMySQL) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (c *vectorTestMySQL) Ping(context.Context) error { return nil }
func (c *vectorTestMySQL) record(query string, args []driver.NamedValue) {
	values := make([]any, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	c.calls = append(c.calls, sqlVectorCall{query, values})
}
func (c *vectorTestMySQL) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	c.record(q, args)
	return driver.RowsAffected(1), c.queryError
}
func (c *vectorTestMySQL) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	c.record(q, args)
	if c.queryError != nil {
		return nil, c.queryError
	}
	if strings.Contains(q, "information_schema.tables") {
		return &vectorTestMySQLRows{columns: []string{"TABLE_NAME"}, values: [][]driver.Value{{"agent_knowledge_3"}, {"agent_knowledge_bad"}, {"agent_knowledge_03"}}}, nil
	}
	if strings.HasPrefix(q, "SELECT cosine_distance") {
		return &vectorTestMySQLRows{columns: []string{"distance"}, values: [][]driver.Value{{float64(0)}}}, nil
	}
	return &vectorTestMySQLRows{columns: []string{"chunk_id", "tenant_id", "knowledge_base_id", "document_id", "attempt", "score"}, values: c.searchRows, err: c.rowError}, nil
}

type vectorTestMySQLRows struct {
	columns []string
	values  [][]driver.Value
	index   int
	err     error
}

func (r *vectorTestMySQLRows) Columns() []string { return r.columns }
func (r *vectorTestMySQLRows) Close() error      { return nil }
func (r *vectorTestMySQLRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		if r.err != nil {
			return r.err
		}
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
func dorisFixture(conn *vectorTestMySQL) *dorisDriver {
	return &dorisDriver{connector: func(*mysql.Config) (driver.Connector, error) { return vectorTestConnector{conn}, nil }}
}

func TestDorisParameterizedScopeAndConnectionCleanup(t *testing.T) {
	conn := &vectorTestMySQL{}
	scope := []string{"tenant' OR TRUE--", "base'--", "doc'--", "attempt'--"}
	adapter := dorisFixture(conn)
	if err := adapter.ReplaceDocument(context.Background(), sqlVectorStore("doris"), scope[0], scope[1], scope[2], scope[3], []port.VectorPoint{{ID: "71", Vector: []float32{1, 0, 0}}}); err != nil {
		t.Fatal(err)
	}
	if !conn.closed {
		t.Fatal("操作完成后连接池未关闭")
	}
	assertScopedSQL(t, conn.calls, scope)
	if !strings.Contains(conn.calls[0].query, "`agent`.`agent_knowledge_3`") {
		t.Fatal("没有按维度创建独立Doris表")
	}
	conn = &vectorTestMySQL{queryError: &mysql.MySQLError{Number: 1105, Message: "private-sql-fixture"}}
	if err := dorisFixture(conn).ReplaceDocument(context.Background(), sqlVectorStore("doris"), "t", "b", "d", "a", []port.VectorPoint{{ID: "71", Vector: []float32{1, 0, 0}}}); err == nil || strings.Contains(err.Error(), "private-sql-fixture") || !conn.closed {
		t.Fatal("SQL失败未脱敏或连接未释放")
	}
}
func TestDorisSearchAndAttemptCleanupScope(t *testing.T) {
	conn := &vectorTestMySQL{searchRows: [][]driver.Value{{"71", "tenant", "base", "doc", "attempt", float64(0.8)}}}
	adapter := dorisFixture(conn)
	got, err := adapter.Search(context.Background(), sqlVectorStore("doris"), "tenant", "base", []float32{1, 0, 0}, 3)
	if err != nil || len(got) != 1 || got[0].ID != "71" {
		t.Fatalf("有效Doris命中丢失: %+v %v", got, err)
	}
	conn = &vectorTestMySQL{searchRows: [][]driver.Value{{"71", "foreign", "base", "doc", "attempt", float64(0.8)}}}
	if _, err := dorisFixture(conn).Search(context.Background(), sqlVectorStore("doris"), "tenant", "base", []float32{1, 0, 0}, 3); err == nil {
		t.Fatal("接受了跨企业Doris命中")
	}
	conn = &vectorTestMySQL{}
	if err := dorisFixture(conn).DeleteAttempt(context.Background(), sqlVectorStore("doris"), "tenant", "base", "doc", "old-attempt"); err != nil {
		t.Fatal(err)
	}
	deletes := 0
	for _, call := range conn.calls {
		if strings.HasPrefix(call.query, "DELETE FROM") {
			deletes++
			if !strings.Contains(call.query, "AND attempt=?") || !reflect.DeepEqual(call.args, []any{"tenant", "base", "doc", "old-attempt"}) {
				t.Fatal("Doris旧处理代删除范围被扩展")
			}
		}
	}
	if deletes != 1 {
		t.Fatal("Doris枚举表没有排除非维度表")
	}
}

func TestSQLVectorIdentifiersAndCanceledErrors(t *testing.T) {
	for _, provider := range []string{"pgvector", "doris"} {
		for _, field := range []string{"table", "database", "host", "port"} {
			store := sqlVectorStore(provider)
			store.Config[field] = "a`; DROP TABLE users;--"
			var err error
			if provider == "pgvector" {
				err = validatePGVectorStore(store)
				if field == "database" {
					continue
				}
			} else {
				err = validateDorisStore(store)
			}
			if err == nil {
				t.Fatalf("危险标识通过校验: %s/%s", provider, field)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(vectorSQLError(ctx, "查询", errors.New("private-sql-fixture")), context.Canceled) || !errors.Is(dorisError(ctx, "查询", errors.New("private-sql-fixture")), context.Canceled) {
		t.Fatal("SQL错误没有保留取消语义")
	}
}
