package knowledge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
)

//go:embed assets/doris.svg
var dorisLogo []byte

// DorisEntry 通过 MySQL 协议管理独立的维度表，使用 Doris 原生精确余弦距离。
func DorisEntry() Entry {
	return Entry{Definition: dto.VectorProvider{ID: "doris", Name: "Apache Doris", Description: "Apache Doris · MySQL 协议 · 精确余弦检索", Logo: "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(dorisLogo), Protocol: "doris", Fields: []dto.VectorField{
		{Key: "host", Label: "主机", Type: "text", Required: true, Immutable: true, Default: "host.docker.internal"},
		{Key: "port", Label: "SQL 端口", Type: "number", Required: true, Immutable: true, Default: "9030", Help: "Doris FE 的 MySQL 协议端口。"},
		{Key: "database", Label: "数据库", Type: "text", Required: true, Immutable: true, Default: "agent", Help: "请先创建目标数据库；驱动仅创建自己的索引表。"},
		{Key: "username", Label: "用户名", Type: "text", Required: true, Immutable: true, Default: "root"},
		{Key: "password", Label: "密码", Type: "password", Secret: true},
		{Key: "table", Label: "表前缀", Type: "text", Required: true, Immutable: true, Default: "agent_knowledge", Help: "按向量维度创建独立表，需具备建表、写入与查询权限。"},
	}}, Driver: &dorisDriver{}, Validate: validateDorisStore}
}

type dorisConfig struct {
	host, database, username, password, table string
	port                                      uint16
}

func decodeDoris(store domain.VectorStore) (dorisConfig, error) {
	c := dorisConfig{host: store.Config["host"], database: store.Config["database"], username: store.Config["username"], password: store.Config["password"], table: store.Config["table"]}
	port, err := strconv.ParseUint(store.Config["port"], 10, 16)
	if err != nil || port == 0 || c.host == "" || len(c.host) > 253 || strings.ContainsAny(c.host, " /\\@?&#\x00\r\n") || c.username == "" || len(c.username) > 256 || strings.ContainsRune(c.username, 0) || !sqlVectorName.MatchString(c.database) || !sqlVectorName.MatchString(c.table) {
		return c, saas.Invalid("Doris 主机、端口、数据库、用户名或表前缀无效")
	}
	c.port = uint16(port)
	return c, nil
}
func validateDorisStore(store domain.VectorStore) error { _, err := decodeDoris(store); return err }

type dorisDriver struct {
	connector func(*mysql.Config) (driver.Connector, error)
}

func (d *dorisDriver) connect(ctx context.Context, store domain.VectorStore) (*sql.DB, dorisConfig, error) {
	c, err := decodeDoris(store)
	if err != nil {
		return nil, c, err
	}
	config := mysql.NewConfig()
	config.User = c.username
	config.Passwd = c.password
	config.Net = "tcp"
	config.Addr = net.JoinHostPort(c.host, strconv.Itoa(int(c.port)))
	config.DBName = c.database
	config.Timeout = 10 * time.Second
	config.ReadTimeout = 2 * time.Minute
	config.WriteTimeout = 2 * time.Minute
	var connector driver.Connector
	if d.connector != nil {
		connector, err = d.connector(config)
	} else {
		connector, err = mysql.NewConnector(config)
	}
	if err != nil {
		return nil, c, errors.New("Doris 连接配置无效")
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(2 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, c, dorisError(ctx, "连接", err)
	}
	return db, c, nil
}
func dorisError(ctx context.Context, operation string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var upstream *mysql.MySQLError
	if errors.As(err, &upstream) {
		return fmt.Errorf("Doris %s失败（错误码 %d）", operation, upstream.Number)
	}
	return fmt.Errorf("Doris %s失败", operation)
}

// Test 检查目标数据库及原生向量距离函数，不创建索引表。
func (d *dorisDriver) Test(ctx context.Context, store domain.VectorStore) error {
	db, _, err := d.connect(ctx, store)
	if err != nil {
		return err
	}
	defer db.Close()
	var distance float64
	if err := db.QueryRowContext(ctx, `SELECT cosine_distance([1.0,0.0],[1.0,0.0])`).Scan(&distance); err != nil {
		return dorisError(ctx, "向量函数检查", err)
	}
	if distance != 0 {
		return errors.New("Doris 返回了无效的余弦距离")
	}
	return nil
}
func dorisTable(c dorisConfig, dimension int) string {
	return "`" + c.database + "`.`" + c.table + "_" + strconv.Itoa(dimension) + "`"
}

func (d *dorisDriver) ReplaceDocument(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, points []port.VectorPoint) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, true); err != nil {
		return err
	}
	dimension, err := vectorPoints(points)
	if err != nil || dimension == 0 {
		return err
	}
	db, c, err := d.connect(ctx, store)
	if err != nil {
		return err
	}
	defer db.Close()
	table := dorisTable(c, dimension)
	// UNIQUE KEY 以完整范围派生的稳定 ID 实现重试覆盖；不依赖 ANN 索引版本差异。
	ddl := `CREATE TABLE IF NOT EXISTS ` + table + ` (id VARCHAR(64) NOT NULL,tenant_id VARCHAR(256) NOT NULL,knowledge_base_id VARCHAR(256) NOT NULL,document_id VARCHAR(256) NOT NULL,attempt VARCHAR(256) NOT NULL,chunk_id VARCHAR(256) NOT NULL,embedding ARRAY<FLOAT> NOT NULL) ENGINE=OLAP UNIQUE KEY(id) DISTRIBUTED BY HASH(id) BUCKETS 1 PROPERTIES ("replication_num"="1","enable_unique_key_merge_on_write"="true")`
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return dorisError(ctx, "建表", err)
	}
	// Doris 的 DELETE 与 INSERT 不组成跨语句事务；失败处理代不会被应用层公开。
	// 只替换本处理代，不能先删除仍有效的旧代。
	if _, err := db.ExecContext(ctx, `DELETE FROM `+table+` WHERE tenant_id=? AND knowledge_base_id=? AND document_id=? AND attempt=?`, tenant, base, document, attempt); err != nil {
		return dorisError(ctx, "清理当前处理代", err)
	}
	for start := 0; start < len(points); start += 64 {
		values := make([]string, 0, min(64, len(points)-start))
		args := make([]any, 0, 6*cap(values))
		for _, point := range points[start:min(start+64, len(points))] {
			// MySQL 驱动不支持 ARRAY 参数；只有已验证有限 float32 参与字面量序列化。
			values = append(values, "(?,?,?,?,?,?,"+vectorLiteral(point.Vector)+")")
			args = append(args, vectorUUID(tenant, base, document, attempt, point.ID), tenant, base, document, attempt, point.ID)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO `+table+` (id,tenant_id,knowledge_base_id,document_id,attempt,chunk_id,embedding) VALUES `+strings.Join(values, ","), args...); err != nil {
			return dorisError(ctx, "写入向量", err)
		}
	}
	return nil
}
func dorisTables(ctx context.Context, db *sql.DB, c dorisConfig) ([]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT TABLE_NAME FROM information_schema.tables WHERE TABLE_SCHEMA=? AND TABLE_NAME LIKE ?`, c.database, c.table+"\\_%")
	if err != nil {
		return nil, dorisError(ctx, "枚举索引表", err)
	}
	defer rows.Close()
	dimensions := []int{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, dorisError(ctx, "读取索引表", err)
		}
		dim, e := strconv.Atoi(strings.TrimPrefix(name, c.table+"_"))
		if e == nil && dim > 0 && dim <= 65536 && name == c.table+"_"+strconv.Itoa(dim) {
			dimensions = append(dimensions, dim)
		}
		if len(dimensions) > 65536 {
			return nil, errors.New("Doris 索引表数量超过限制")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, dorisError(ctx, "枚举索引表", err)
	}
	return dimensions, nil
}
func (d *dorisDriver) DeleteDocument(ctx context.Context, store domain.VectorStore, tenant, base, document string) error {
	return d.delete(ctx, store, tenant, base, document, "")
}
func (d *dorisDriver) DeleteAttempt(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	if attempt == "" {
		return errors.New("清理索引必须指定处理代")
	}
	return d.delete(ctx, store, tenant, base, document, attempt)
}
func (d *dorisDriver) delete(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	if err := vectorRequiredScope(store, tenant, base, document, attempt, attempt != ""); err != nil {
		return err
	}
	db, c, err := d.connect(ctx, store)
	if err != nil {
		return err
	}
	defer db.Close()
	dims, err := dorisTables(ctx, db, c)
	if err != nil {
		return err
	}
	for _, dim := range dims {
		query := `DELETE FROM ` + dorisTable(c, dim) + ` WHERE tenant_id=? AND knowledge_base_id=? AND document_id=?`
		args := []any{tenant, base, document}
		if attempt != "" {
			query += ` AND attempt=?`
			args = append(args, attempt)
		}
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			return dorisError(ctx, "清理索引", err)
		}
	}
	return nil
}
func (d *dorisDriver) Search(ctx context.Context, store domain.VectorStore, tenant, base string, query []float32, limit int) ([]port.VectorHit, error) {
	if err := vectorSearchInput(tenant, base, query, limit); err != nil {
		return nil, err
	}
	if err := vectorRequiredScope(store, tenant, base, "search", "", false); err != nil {
		return nil, err
	}
	db, c, err := d.connect(ctx, store)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	dims, err := dorisTables(ctx, db, c)
	if err != nil {
		return nil, err
	}
	exists := false
	for _, dim := range dims {
		if dim == len(query) {
			exists = true
		}
	}
	if !exists {
		return []port.VectorHit{}, nil
	}
	statement := `SELECT chunk_id,tenant_id,knowledge_base_id,document_id,attempt,1-cosine_distance(embedding,` + vectorLiteral(query) + `) AS score FROM ` + dorisTable(c, len(query)) + ` WHERE tenant_id=? AND knowledge_base_id=? ORDER BY score DESC LIMIT ?`
	rows, err := db.QueryContext(ctx, statement, tenant, base, limit)
	if err != nil {
		return nil, dorisError(ctx, "检索", err)
	}
	defer rows.Close()
	hits := []port.VectorHit{}
	for rows.Next() {
		var id, t, b, doc, a string
		var score float64
		if err := rows.Scan(&id, &t, &b, &doc, &a, &score); err != nil {
			return nil, dorisError(ctx, "读取命中", err)
		}
		metadata := vectorScope(t, b, doc, a)
		metadata["chunk_id"] = id
		hit, err := vectorScopedHit(metadata, tenant, base, id, score)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, dorisError(ctx, "检索", err)
	}
	return hits, nil
}

var _ port.Vectors = (*dorisDriver)(nil)
