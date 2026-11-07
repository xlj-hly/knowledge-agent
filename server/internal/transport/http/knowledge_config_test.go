package httpapi

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVectorInputRejectsNonStringConfigAndNull(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `{}`, `{"config":null}`, `{"config":[]}`, `{"config":{"api_key":null}}`, `{"config":{"tls":true}}`, `{"config":{"port":6333}}`, `{"config":{},"tenant_id":"10"}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		if _, err := decodeVectorStore(c); err == nil {
			t.Fatalf("非法向量配置被绑定为有效输入: %s", body)
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"连接","provider":"qdrant","config":{"endpoint":"http://localhost","api_key":"","tls":"false"}}`))
	got, err := decodeVectorStore(c)
	if err != nil {
		t.Fatal(err)
	}
	if value, present := got.Config["api_key"]; !present || value != "" || got.Config["tls"] != "false" {
		t.Fatal("显式清空秘密或布尔字符串语义丢失")
	}
}
