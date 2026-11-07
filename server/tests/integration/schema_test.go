package saas_test

import (
	"context"
	"testing"
	"time"
)

func TestSchemaContainsImplementedTablesAndComments(t *testing.T) {
	s := newIntegrationSuite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var tables string
	if err := s.pool.QueryRow(ctx, `
		SELECT string_agg(c.relname, ',' ORDER BY c.relname)
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind = 'r'
	`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != "ai_models,ai_settings,auth_sessions,chat_messages,chat_sessions,company,knowledge_bases,knowledge_chunks,knowledge_documents,knowledge_vector_stores,super_admin,users" {
		t.Fatalf("SaaS schema contains unexpected tables: %s", tables)
	}
	for table, names := range map[string][]string{
		"company": {"app_id", "app_secret", "wechat_config_version"},
		"users":   {"wechat_app_id", "open_id"},
	} {
		for _, name := range names {
			if s.count(t, "SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2", table, name) != 1 {
				t.Fatalf("%s.%s was not merged into its owner table", table, name)
			}
		}
	}
	for name, query := range map[string]string{
		"table comments":                    `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relkind='r' AND coalesce(obj_description(c.oid, 'pg_class'), '')=''`,
		"column comments":                   `SELECT count(*) FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relkind='r' AND a.attnum>0 AND NOT a.attisdropped AND coalesce(col_description(c.oid,a.attnum), '')=''`,
		"database relationship constraints": `SELECT count(*) FROM pg_constraint c JOIN pg_namespace n ON n.oid=c.connamespace WHERE n.nspname=current_schema() AND c.contype IN ('f','c')`,
		"row level security":                `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relkind='r' AND c.relrowsecurity`,
		"user triggers":                     `SELECT count(*) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND NOT t.tgisinternal`,
	} {
		var count int
		if err := s.pool.QueryRow(ctx, query).Scan(&count); err != nil {
			t.Fatalf("inspect %s: %v", name, err)
		}
		if count != 0 {
			t.Errorf("SaaS schema has %d violations for %s", count, name)
		}
	}
}
