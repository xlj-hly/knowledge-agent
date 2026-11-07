package repository

import (
	"reflect"
	"testing"

	"go-server/internal/domain/knowledge"
)

func TestDocumentCleanupRoundTripRetainsOriginalStore(t *testing.T) {
	d := knowledge.Document{ID: "21", TenantID: "10", BaseID: "11", Attempt: "new", IndexStoreID: "31", CleanupTargets: []knowledge.CleanupTarget{{StoreID: "30", Attempt: "old"}}}
	row, err := documentRecord(d)
	if err != nil {
		t.Fatal(err)
	}
	if row.CleanupTargets != `[{"store_id":"30","attempt":"old"}]` {
		t.Fatalf("清理目标的持久化格式变化: %s", row.CleanupTargets)
	}
	got, err := row.view()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, d) {
		t.Fatalf("索引迁移丢失清理归属: %+v", got)
	}
}

func TestMalformedCleanupStateCannotBecomeEmptyList(t *testing.T) {
	if _, err := (documentRow{CleanupTargets: `[{"store_id":`}).view(); err == nil {
		t.Fatal("损坏的旧索引清理记录被当成空清单")
	}
}

func TestVectorConfigRoundTripPreservesArbitraryEngineFields(t *testing.T) {
	store := knowledge.VectorStore{ID: "31", TenantID: "10", Name: "另一引擎", Provider: "custom", Config: map[string]string{"endpoint": "http://localhost", "database": "docs", "tls": "false", "port": "6334", "api_key": "private"}}
	row, err := vectorRecord(store)
	if err != nil {
		t.Fatal(err)
	}
	got, err := row.view()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, store) {
		t.Fatal("持久化丢失了引擎专属配置")
	}
	for _, bad := range []string{`null`, `[]`, `{"api_key":null}`, `{"port":6334}`, `{"tls":true}`} {
		if _, err := (vectorRow{Config: bad}).view(); err == nil {
			t.Fatalf("非法配置没有报告扫描错误: %s", bad)
		}
	}
}
