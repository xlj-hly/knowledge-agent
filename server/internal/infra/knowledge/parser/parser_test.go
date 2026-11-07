package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func docxFixture(t *testing.T, name, content string) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	f, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestParseDOCXRetainsParagraphsAndTableText(t *testing.T) {
	data := docxFixture(t, "word/document.xml", `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>七日退款</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>联系电话</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>4000001234</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`)
	text, err := Parse(context.Background(), "guide.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "七日退款\n\n") || !strings.Contains(text, "联系电话") || !strings.Contains(text, "4000001234") {
		t.Fatalf("DOCX content lost: %q", text)
	}
}

func TestParseRejectsBrokenDOCXAndOversizedExpansion(t *testing.T) {
	cases := [][]byte{[]byte("not a zip"), docxFixture(t, "other.xml", "empty"), docxFixture(t, "word/document.xml", `<document><t>broken`), docxFixture(t, "word/document.xml", `<document><t>`+strings.Repeat("a", maxTextBytes+1)+`</t></document>`)}
	for i, data := range cases {
		if _, err := Parse(context.Background(), "bad.docx", data); err == nil {
			t.Errorf("invalid DOCX %d accepted", i)
		}
	}
}

func TestParseWindowsTextEncodings(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"UTF8 BOM", []byte("\ufeff中文"), "中文"},
		{"UTF16 LE", []byte{0xff, 0xfe, 0x2d, 0x4e, 0x87, 0x65}, "中文"},
		{"GBK", []byte{0xd6, 0xd0, 0xce, 0xc4}, "中文"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := Parse(context.Background(), "file.txt", tc.data)
			if err != nil || text != tc.want {
				t.Fatalf("decoded %q, err=%v", text, err)
			}
		})
	}
}

func TestParseRejectsExcessiveJSONNestingAndTextSize(t *testing.T) {
	deep := strings.Repeat(`{"nested":`, 129) + `"value"` + strings.Repeat("}", 129)
	if _, err := Parse(context.Background(), "deep.json", []byte(deep)); err == nil {
		t.Fatal("excessively nested JSON accepted")
	}
	if _, err := Parse(context.Background(), "large.txt", []byte(strings.Repeat("a", maxTextBytes+1))); err == nil {
		t.Fatal("oversized text accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Parse(ctx, "cancelled.txt", []byte("content")); err == nil {
		t.Fatal("cancelled parser continued")
	}
}

func TestParseRejectsJSONPathExpansion(t *testing.T) {
	leaves := make(map[string]string, 100)
	for i := range 100 {
		leaves[strings.Repeat("x", i+1)] = strings.Repeat("value", 20)
	}
	// 长包装键在每个语义分块重复，源文件很小，派生正文仍须受总量上限约束。
	data, err := json.Marshal(map[string]any{strings.Repeat("wrapper", 15000): leaves})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) >= maxTextBytes {
		t.Fatal("fixture must be below input limit")
	}
	if _, err := Parse(context.Background(), "expanded.json", data); err == nil {
		t.Fatal("unbounded JSON path expansion accepted")
	}
}
