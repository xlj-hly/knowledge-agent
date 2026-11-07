package knowledge

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
)

//go:embed assets/pgvector.png
var pgvectorLogo []byte

// PGVectorEntry 使用已安装 pgvector 扩展的 PostgreSQL，不复用 SaaS 业务连接。
func PGVectorEntry() Entry {
	return Entry{Definition: dto.VectorProvider{ID: "pgvector", Name: "PGVector", Description: "PostgreSQL pgvector · 独立连接 · COSINE", Logo: "data:image/png;base64," + base64.StdEncoding.EncodeToString(pgvectorLogo), Protocol: "pgvector", Fields: []dto.VectorField{
		{Key: "host", Label: "主机", Type: "text", Required: true, Immutable: true, Default: "host.docker.internal"},
		{Key: "port", Label: "端口", Type: "number", Required: true, Immutable: true, Default: "5432"},
		{Key: "database", Label: "数据库", Type: "text", Required: true, Immutable: true, Default: "agent", Help: "需预先安装 vector 扩展。"},
		{Key: "username", Label: "用户名", Type: "text", Required: true, Immutable: true, Default: "postgres"},
		{Key: "password", Label: "密码", Type: "password", Secret: true},
		{Key: "sslmode", Label: "TLS 模式", Type: "select", Required: true, Default: "prefer", Options: []dto.VectorFieldOption{{Value: "disable", Label: "禁用"}, {Value: "prefer", Label: "优先 TLS"}, {Value: "require", Label: "必须 TLS"}, {Value: "verify-full", Label: "验证证书与主机"}}},
		{Key: "schema", Label: "Schema", Type: "text", Required: true, Immutable: true, Default: "public"},
		{Key: "table", Label: "表前缀", Type: "text", Required: true, Immutable: true, Default: "agent_knowledge"},
	}}, Driver: &pgvectorDriver{}, Validate: validatePGVectorStore}
}

var sqlVectorName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,47}$`)

type pgvectorConfig struct {
	host, database, username, password, sslmode, schema, table string
	port                                                       uint16
}

func decodePGVector(store domain.VectorStore) (pgvectorConfig, error) {
	c := pgvectorConfig{host: store.Config["host"], database: store.Config["database"], username: store.Config["username"], password: store.Config["password"], sslmode: store.Config["sslmode"], schema: store.Config["schema"], table: store.Config["table"]}
	port, err := strconv.ParseUint(store.Config["port"], 10, 16)
	if err != nil || port == 0 || c.host == "" || strings.ContainsAny(c.host, " /\\@?&#\x00\r\n") || len(c.host) > 253 || c.username == "" || c.database == "" || strings.ContainsRune(c.database+c.username, 0) {
		return c, saas.Invalid("PGVector 主机、端口、数据库或用户名无效")
	}
	if !sqlVectorName.MatchString(c.schema) || !sqlVectorName.MatchString(c.table) {
		return c, saas.Invalid("PGVector Schema 和表前缀只能使用字母、数字与下划线")
	}
	switch c.sslmode {
	case "disable", "prefer", "require", "verify-full":
	default:
		return c, saas.Invalid("PGVector TLS 模式无效")
	}
	c.port = uint16(port)
	return c, nil
}

func validatePGVectorStore(store domain.VectorStore) error {
	_, err := decodePGVector(store)
	return err
}

type pgvectorConnection interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Begin(context.Context) (pgx.Tx, error)
	Close(context.Context) error
}

type pgvectorDriver struct {
	dial func(context.Context, *pgx.ConnConfig) (pgvectorConnection, error)
}

func (v *pgvectorDriver) connect(ctx context.Context, store domain.VectorStore) (pgvectorConnection, pgvectorConfig, error) {
	c, err := decodePGVector(store)
	if err != nil {
		return nil, c, err
	}
	address := &url.URL{Scheme: "postgresql", Host: net.JoinHostPort(c.host, strconv.Itoa(int(c.port))), Path: "/" + c.database, User: url.UserPassword(c.username, c.password)}
	query := url.Values{"sslmode": {c.sslmode}, "connect_timeout": {"10"}}
	address.RawQuery = query.Encode()
	config, err := pgx.ParseConfig(address.String())
	if err != nil {
		return nil, c, errors.New("PGVector 连接配置无效")
	}
	config.Password = c.password
	config.RuntimeParams["statement_timeout"] = "120000"
	var conn pgvectorConnection
	if v.dial != nil {
		conn, err = v.dial(ctx, config)
	} else {
		conn, err = pgx.ConnectConfig(ctx, config)
	}
	if err != nil {
		return nil, c, vectorSQLError(ctx, "PGVector 连接", err)
	}
	return conn, c, nil
}

func closePGVector(conn pgvectorConnection) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = conn.Close(ctx)
}

func vectorSQLError(ctx context.Context, operation string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return fmt.Errorf("%s失败（SQLSTATE %s）", operation, pg.Code)
	}
	return fmt.Errorf("%s失败", operation)
}

func (v *pgvectorDriver) Test(ctx context.Context, store domain.VectorStore) error {
	conn, _, err := v.connect(ctx, store)
	if err != nil {
		return err
	}
	defer closePGVector(conn)
	var version string
	if err := conn.QueryRow(ctx, "SELECT extversion FROM pg_extension WHERE extname = 'vector'").Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("目标数据库未安装 pgvector 扩展")
		}
		return vectorSQLError(ctx, "PGVector 扩展检查", err)
	}
	return nil
}

func pgvectorTable(c pgvectorConfig, dimension int) string {
	return pgx.Identifier{c.schema, c.table + "_" + strconv.Itoa(dimension)}.Sanitize()
}

func vectorLiteral(vector []float32) string {
	parts := make([]string, len(vector))
	for i, value := range vector {
		parts[i] = strconv.FormatFloat(float64(value), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func (v *pgvectorDriver) ReplaceDocument(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, points []port.VectorPoint) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, true); err != nil {
		return err
	}
	dim, err := vectorPoints(points)
	if err != nil || dim == 0 {
		return err
	}
	if dim > 16000 {
		return errors.New("PGVector向量维度不能超过16000")
	}
	conn, c, err := v.connect(ctx, store)
	if err != nil {
		return err
	}
	defer closePGVector(conn)
	table := pgvectorTable(c, dim)
	// 每维度独立表保留 embedding 维度约束；不创建或迁移企业业务表。
	ddl := `CREATE TABLE IF NOT EXISTS ` + table + ` (id text PRIMARY KEY,tenant_id text NOT NULL,knowledge_base_id text NOT NULL,document_id text NOT NULL,attempt text NOT NULL,chunk_id text NOT NULL,embedding vector(` + strconv.Itoa(dim) + `) NOT NULL)`
	if _, err := conn.Exec(ctx, ddl); err != nil {
		return vectorSQLError(ctx, "PGVector 建表", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return vectorSQLError(ctx, "PGVector 开启写入", err)
	}
	// 请求取消后仍需回滚；独立的短期限防止清理无限等待。
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	// 回收范围只包含当前处理代；旧代必须由应用层在确认新代后显式删除。
	if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE tenant_id=$1 AND knowledge_base_id=$2 AND document_id=$3 AND attempt=$4`, tenant, base, document, attempt); err != nil {
		return vectorSQLError(ctx, "PGVector 清理当前处理代", err)
	}
	for start := 0; start < len(points); start += 64 {
		batch := &pgx.Batch{}
		for _, point := range points[start:min(start+64, len(points))] {
			batch.Queue(`INSERT INTO `+table+` (id,tenant_id,knowledge_base_id,document_id,attempt,chunk_id,embedding) VALUES ($1,$2,$3,$4,$5,$6,$7::vector)`, vectorUUID(tenant, base, document, attempt, point.ID), tenant, base, document, attempt, point.ID, vectorLiteral(point.Vector))
		}
		results := tx.SendBatch(ctx, batch)
		if err := results.Close(); err != nil {
			return vectorSQLError(ctx, "PGVector 批量写入", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return vectorSQLError(ctx, "PGVector 提交写入", err)
	}
	return nil
}

func pgvectorTables(ctx context.Context, conn pgvectorConnection, c pgvectorConfig) ([]string, error) {
	rows, err := conn.Query(ctx, `SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname=$1 AND starts_with(tablename,$2)`, c.schema, c.table+"_")
	if err != nil {
		return nil, vectorSQLError(ctx, "PGVector 枚举索引表", err)
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, vectorSQLError(ctx, "PGVector 读取索引表", err)
		}
		dim, err := strconv.Atoi(strings.TrimPrefix(name, c.table+"_"))
		if err == nil && dim > 0 && dim <= 65536 && name == c.table+"_"+strconv.Itoa(dim) {
			names = append(names, name)
		}
		if len(names) > 65536 {
			return nil, errors.New("PGVector 索引表数量超过限制")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, vectorSQLError(ctx, "PGVector 枚举索引表", err)
	}
	return names, nil
}

func (v *pgvectorDriver) DeleteDocument(ctx context.Context, store domain.VectorStore, tenant, base, document string) error {
	return v.delete(ctx, store, tenant, base, document, "")
}
func (v *pgvectorDriver) DeleteAttempt(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	if attempt == "" {
		return errors.New("清理索引必须指定处理代")
	}
	return v.delete(ctx, store, tenant, base, document, attempt)
}
func (v *pgvectorDriver) delete(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, attempt != ""); err != nil {
		return err
	}
	conn, c, err := v.connect(ctx, store)
	if err != nil {
		return err
	}
	defer closePGVector(conn)
	names, err := pgvectorTables(ctx, conn, c)
	if err != nil {
		return err
	}
	for _, name := range names {
		table := pgx.Identifier{c.schema, name}.Sanitize()
		sql := `DELETE FROM ` + table + ` WHERE tenant_id=$1 AND knowledge_base_id=$2 AND document_id=$3`
		args := []any{tenant, base, document}
		if attempt != "" {
			sql += ` AND attempt=$4`
			args = append(args, attempt)
		}
		if _, err := conn.Exec(ctx, sql, args...); err != nil {
			return vectorSQLError(ctx, "PGVector 清理索引", err)
		}
	}
	return nil
}

func (v *pgvectorDriver) Search(ctx context.Context, store domain.VectorStore, tenant, base string, query []float32, limit int) ([]port.VectorHit, error) {
	if err := vectorSearchInput(tenant, base, query, limit); err != nil {
		return nil, err
	}
	if len(query) > 16000 {
		return nil, errors.New("PGVector向量维度不能超过16000")
	}
	if err := vectorRequiredScope(store, tenant, base, "search", "", false); err != nil {
		return nil, err
	}
	conn, c, err := v.connect(ctx, store)
	if err != nil {
		return nil, err
	}
	defer closePGVector(conn)
	table := pgvectorTable(c, len(query))
	var exists *string
	if err := conn.QueryRow(ctx, `SELECT to_regclass($1)::text`, table).Scan(&exists); err != nil {
		return nil, vectorSQLError(ctx, "PGVector 检查索引表", err)
	}
	if exists == nil {
		return []port.VectorHit{}, nil
	}
	rows, err := conn.Query(ctx, `SELECT chunk_id,tenant_id,knowledge_base_id,document_id,attempt,1-(embedding <=> $1::vector) AS score FROM `+table+` WHERE tenant_id=$2 AND knowledge_base_id=$3 ORDER BY embedding <=> $1::vector LIMIT $4`, vectorLiteral(query), tenant, base, limit)
	if err != nil {
		return nil, vectorSQLError(ctx, "PGVector 检索", err)
	}
	defer rows.Close()
	hits := []port.VectorHit{}
	for rows.Next() {
		var id, t, b, d, a string
		var score float64
		if err := rows.Scan(&id, &t, &b, &d, &a, &score); err != nil {
			return nil, vectorSQLError(ctx, "PGVector 读取命中", err)
		}
		metadata := vectorScope(t, b, d, a)
		metadata["chunk_id"] = id
		hit, err := vectorScopedHit(metadata, tenant, base, id, score)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, vectorSQLError(ctx, "PGVector 检索", err)
	}
	return hits, nil
}

var _ port.Vectors = (*pgvectorDriver)(nil)
