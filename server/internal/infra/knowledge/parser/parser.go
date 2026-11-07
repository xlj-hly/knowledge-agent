// Package parser 将支持的本地文档格式转换为可切分的 UTF-8 正文。
package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

const maxTextBytes = 2 << 20

// Parse 解析本地支持的文件；PDF 由上层适配到 DocReader 服务。
func Parse(ctx context.Context, filename string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(data) > 10<<20 {
		return "", errors.New("资料大小不能超过 10 MiB")
	}
	var text string
	var err error
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".txt", ".text", ".md", ".markdown":
		text = string(decodeText(data))
	case ".csv":
		text, err = csvToMarkdownContext(ctx, decodeText(data), maxTextBytes)
	case ".json":
		if len(data) > maxTextBytes {
			return "", errors.New("JSON 正文不能超过 2 MiB")
		}
		if err := validateJSONDepth(ctx, data); err != nil {
			return "", err
		}
		text, err = jsonToMarkdown(data)
	case ".docx":
		text, err = parseDOCX(ctx, data)
	default:
		return "", errors.New("不支持的本地文档格式")
	}
	if err != nil {
		return "", fmt.Errorf("解析资料失败: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	text = strings.TrimPrefix(text, "\ufeff")
	if len(text) > maxTextBytes {
		return "", errors.New("解析后的正文不能超过 2 MiB")
	}
	if strings.ContainsRune(text, 0) {
		return "", errors.New("正文包含无效的空字符")
	}
	return text, nil
}

func validateJSONDepth(ctx context.Context, data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(trimBOM(data)))
	decoder.UseNumber()
	depth := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("JSON 格式无效")
		}
		if delimiter, ok := token.(json.Delim); ok {
			if delimiter == '{' || delimiter == '[' {
				depth++
				// WeKnora 转换器递归保留 JSON 路径，显式限制嵌套防止深层输入放大栈和路径开销。
				if depth > 128 {
					return errors.New("JSON 嵌套层数不能超过 128")
				}
			} else {
				depth--
			}
		}
	}
}

func parseDOCX(ctx context.Context, data []byte) (string, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("DOCX 压缩包格式无效")
	}
	if len(z.File) > 2000 {
		return "", errors.New("DOCX 包内文件数量过多")
	}
	for _, file := range z.File {
		if file.Name != "word/document.xml" {
			continue
		}
		// 仅解压正文 XML，不写文件；限制解压后大小以约束压缩包扩张。
		const maxXMLBytes = 8 << 20
		if file.UncompressedSize64 > maxXMLBytes {
			return "", errors.New("DOCX 正文 XML 过大")
		}
		r, err := file.Open()
		if err != nil {
			return "", errors.New("无法读取 DOCX 正文")
		}
		body, readErr := io.ReadAll(io.LimitReader(r, maxXMLBytes+1))
		closeErr := r.Close()
		if readErr != nil || closeErr != nil {
			return "", errors.New("读取 DOCX 正文失败")
		}
		if len(body) > maxXMLBytes {
			return "", errors.New("DOCX 正文 XML 过大")
		}
		decoder := xml.NewDecoder(bytes.NewReader(body))
		var out strings.Builder
		inText := false
		for {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			token, err := decoder.Token()
			if err == io.EOF {
				return out.String(), nil
			}
			if err != nil {
				return "", errors.New("DOCX 正文 XML 无效")
			}
			switch t := token.(type) {
			case xml.StartElement:
				switch t.Name.Local {
				case "t":
					inText = true
				case "tab":
					out.WriteByte('\t')
				case "br", "cr":
					out.WriteByte('\n')
				}
			case xml.EndElement:
				switch t.Name.Local {
				case "t":
					inText = false
				case "p":
					out.WriteString("\n\n")
				case "tc":
					out.WriteByte('\t')
				}
			case xml.CharData:
				if inText {
					out.Write(t)
				}
			}
			if out.Len() > maxTextBytes {
				return "", errors.New("DOCX 正文不能超过 2 MiB")
			}
		}
	}
	return "", errors.New("DOCX 缺少 word/document.xml")
}

// decodeText 复用 WeKnora 的解码顺序，兼容中文 Windows 的 UTF-16 和 GBK 文本。
func decodeText(data []byte) []byte {
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		if text, err := unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewDecoder().Bytes(data); err == nil {
			return text
		}
	}
	if utf8.Valid(data) {
		return data
	}
	text, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
	if err == nil && !bytes.ContainsRune(text, utf8.RuneError) {
		return text
	}
	text, err = charmap.Windows1252.NewDecoder().Bytes(data)
	if err == nil && !bytes.ContainsRune(text, utf8.RuneError) {
		return text
	}
	text, _ = charmap.ISO8859_1.NewDecoder().Bytes(data)
	return text
}

var csvCellReplacer = strings.NewReplacer(`\`, `\\`, "|", `\|`, "\r\n", "<br />", "\n", "<br />", "\r", "<br />")
