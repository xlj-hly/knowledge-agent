package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

// SQL 引擎集成测试仅在显式提供专用测试连接时运行；不读项目凭据文件。
func TestSQLVectorsLiveLifecycle(t *testing.T) {
	for _, provider := range []string{"pgvector", "doris"} {
		t.Run(provider, func(t *testing.T) {
			variable := "TEST_PGVECTOR_CONFIG"
			if provider == "doris" {
				variable = "TEST_DORIS_CONFIG"
			}
			source := os.Getenv(variable)
			if source == "" {
				t.Skip("未显式配置SQL向量测试连接")
			}
			config := map[string]string{}
			if err := json.Unmarshal([]byte(source), &config); err != nil {
				t.Fatal("SQL向量测试连接必须为字符串JSON对象")
			}
			unique := fmt.Sprintf("agent_test_%d", time.Now().UnixNano())
			config["table"] = unique
			store := domain.VectorStore{Name: "真实SQL索引测试", Provider: provider, Config: config}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			var vectors port.Vectors
			if provider == "pgvector" {
				config["schema"] = unique
				adapter := &pgvectorDriver{}
				conn, _, err := adapter.connect(ctx, store)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := conn.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
					closePGVector(conn)
					t.Fatal("测试库安装vector扩展失败")
				}
				if _, err := conn.Exec(ctx, `CREATE SCHEMA `+`"`+unique+`"`); err != nil {
					closePGVector(conn)
					t.Fatal("创建隔离测试schema失败")
				}
				closePGVector(conn)
				t.Cleanup(func() {
					cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
					defer stop()
					conn, _, err := adapter.connect(cleanup, store)
					if err != nil {
						t.Error(err)
						return
					}
					defer closePGVector(conn)
					if _, err := conn.Exec(cleanup, `DROP SCHEMA `+`"`+unique+`" CASCADE`); err != nil {
						t.Error("清理隔离向量测试schema失败")
					}
				})
				vectors = adapter
			} else {
				adapter := &dorisDriver{}
				t.Cleanup(func() {
					cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
					defer stop()
					db, c, err := adapter.connect(cleanup, store)
					if err != nil {
						t.Error(err)
						return
					}
					defer db.Close()
					if _, err := db.ExecContext(cleanup, `DROP TABLE IF EXISTS `+dorisTable(c, 3)); err != nil {
						t.Error("清理Doris测试表失败")
					}
				})
				vectors = adapter
			}
			if err := vectors.Test(ctx, store); err != nil {
				t.Fatal(err)
			}
			points := []port.VectorPoint{{ID: "71", Vector: []float32{1, 0, 0}}}
			if err := vectors.ReplaceDocument(ctx, store, "tenant-a", "base", "doc", "old", points); err != nil {
				t.Fatal(err)
			}
			if err := vectors.ReplaceDocument(ctx, store, "tenant-a", "base", "doc", "new", points); err != nil {
				t.Fatal(err)
			}
			if err := vectors.ReplaceDocument(ctx, store, "tenant-b", "base", "doc", "foreign", []port.VectorPoint{{ID: "99", Vector: []float32{1, 0, 0}}}); err != nil {
				t.Fatal(err)
			}
			hits, err := vectors.Search(ctx, store, "tenant-a", "base", []float32{1, 0, 0}, 10)
			if err != nil || len(hits) != 2 {
				t.Fatalf("两个处理代未独立保存或出现跨企业命中: count=%d err=%v", len(hits), err)
			}
			for _, hit := range hits {
				if hit.ID != "71" || hit.Score < 0.99 {
					t.Fatal("真实余弦检索结果不符合预期")
				}
			}
			if err := vectors.DeleteAttempt(ctx, store, "tenant-a", "base", "doc", "old"); err != nil {
				t.Fatal(err)
			}
			hits, err = vectors.Search(ctx, store, "tenant-a", "base", []float32{1, 0, 0}, 10)
			if err != nil || !reflect.DeepEqual(hits, []port.VectorHit{{ID: "71", Score: 1}}) {
				t.Fatalf("清理旧代误伤当前代: count=%d err=%v", len(hits), err)
			}
			if err := vectors.DeleteDocument(ctx, store, "tenant-a", "base", "doc"); err != nil {
				t.Fatal(err)
			}
			if err := vectors.DeleteDocument(ctx, store, "tenant-a", "base", "doc"); err != nil {
				t.Fatalf("重复删除应幂等: %v", err)
			}
			hits, err = vectors.Search(ctx, store, "tenant-a", "base", []float32{1, 0, 0}, 10)
			if err != nil || len(hits) != 0 {
				t.Fatal("资料删除后仍可检索")
			}
			hits, err = vectors.Search(ctx, store, "tenant-b", "base", []float32{1, 0, 0}, 10)
			if err != nil || len(hits) != 1 || hits[0].ID != "99" {
				t.Fatal("资料删除误伤其他企业")
			}
		})
	}
}
