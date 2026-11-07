package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// defaultJSONChunkSize is the target chunk size in bytes for JSON semantic
// splitting. Approximates 512 tokens (1 token ≈ 3-4 bytes for mixed content).
const defaultJSONChunkSize = 1536

// minJSONChunkSize is the minimum chunk size. A new chunk is only started
// when the current chunk has reached at least this size.
var minJSONChunkSize = defaultJSONChunkSize - 200

// Errors returned by ValidateJSONContent.
var (
	ErrEmptyJSONContent   = errors.New("empty JSON content")
	ErrInvalidJSONContent = errors.New("invalid JSON content")
)

// ValidateJSONContent reports whether data is well-formed JSON after stripping
// a leading UTF-8 BOM. Upload paths call this before enqueueing parse work so
// JSONC / truncated payloads fail immediately instead of after async retries.
func ValidateJSONContent(data []byte) error {
	data = trimBOM(data)
	if len(data) == 0 {
		return ErrEmptyJSONContent
	}
	if !json.Valid(data) {
		return ErrInvalidJSONContent
	}
	return nil
}

// jsonToMarkdown converts raw JSON bytes into markdown text
//
// Key properties:
//   - Every output chunk is a **valid JSON object** (not a fragment).
//   - Nested paths from root to leaf are **fully preserved** in each chunk.
//   - Arrays are converted to index-keyed dicts so the algorithm is uniform.
//   - Small objects that fit within maxChunkSize are kept intact (not split).
//   - The output is a series of fenced ```json code blocks separated by \n\n,
//     which the downstream text chunker can split at block boundaries.
func jsonToMarkdown(data []byte) (string, error) {
	if err := ValidateJSONContent(data); err != nil {
		return "", err
	}
	data = trimBOM(data)

	// UseNumber keeps each number as written. Decoded into a float64, an id
	// above 2^53 changed digits: 12345678901234567890 came out as
	// 12345678901234567000. ValidateJSONContent above has already rejected
	// anything after the value.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var parsed interface{}
	if err := decoder.Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Normalize: convert top-level arrays to index-keyed dicts
	normalized := listToDictPreprocess(parsed)

	// If the whole thing fits in one chunk, just format it
	wholeSize := jsonSize(normalized)
	if wholeSize <= defaultJSONChunkSize {
		return limitedJSONBlock(normalized)
	}

	// Recursive split
	budget := 2
	chunks, err := recursiveJSONSplit(normalized, nil, nil, &budget)
	if err != nil {
		return "", err
	}

	// Convert each chunk dict to a fenced code block
	blocks := make([]string, 0, len(chunks))
	outputBytes := 0
	for _, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}
		block, err := limitedJSONBlock(chunk)
		if err != nil {
			return "", err
		}
		outputBytes += len(block) + 2
		if outputBytes > maxTextBytes {
			return "", errors.New("JSON 转换后的正文不能超过 2 MiB")
		}
		blocks = append(blocks, block)
	}

	if len(blocks) == 0 {
		return limitedJSONBlock(normalized)
	}
	return strings.Join(blocks, "\n\n"), nil
}

// ---------------------------------------------------------------------------
// RecursiveJsonSplitter core algorithm
// ---------------------------------------------------------------------------

// recursiveJSONSplit splits a JSON dict into a list of JSON dicts,
// each fitting within defaultJSONChunkSize. It preserves the full nested
// path from root to each leaf by using setNestedDict.
//
// This is a Go port of LangChain's RecursiveJsonSplitter._json_split.
func recursiveJSONSplit(
	data interface{},
	currentPath []string,
	chunks []map[string]interface{},
	budget *int,
) ([]map[string]interface{}, error) {
	if chunks == nil {
		chunks = []map[string]interface{}{{}}
	}

	dict, ok := data.(map[string]interface{})
	if !ok {
		// Scalar or already-processed value: place it at the current path
		if len(currentPath) > 0 && len(chunks) > 0 {
			if err := setNestedDictBounded(chunks[len(chunks)-1], currentPath, data, budget); err != nil {
				return nil, err
			}
		}
		return chunks, nil
	}

	// Sort keys for deterministic output
	keys := sortedKeys(dict)

	for _, key := range keys {
		value := dict[key]
		newPath := append(append([]string{}, currentPath...), key)

		// Measure sizes
		chunkSize := jsonSize(chunks[len(chunks)-1])
		itemSize := jsonSize(map[string]interface{}{key: value})
		remaining := defaultJSONChunkSize - chunkSize

		if itemSize <= remaining {
			// Item fits in the current chunk — add it preserving the path
			if err := setNestedDictBounded(chunks[len(chunks)-1], newPath, value, budget); err != nil {
				return nil, err
			}
		} else {
			// Item doesn't fit
			if chunkSize >= minJSONChunkSize {
				// Current chunk is big enough, start a new one
				chunks = append(chunks, map[string]interface{}{})
				*budget += 2
			}

			// Check if the value itself is a dict/list that can be recursed into
			normalized := listToDictPreprocess(value)
			if subDict, isDict := normalized.(map[string]interface{}); isDict && canSplitDict(subDict) {
				// Recurse into the sub-object
				var err error
				chunks, err = recursiveJSONSplit(subDict, newPath, chunks, budget)
				if err != nil {
					return nil, err
				}
			} else {
				// Cannot split further (scalar or single-key dict) — place as-is
				if err := setNestedDictBounded(chunks[len(chunks)-1], newPath, value, budget); err != nil {
					return nil, err
				}
			}
		}
	}

	return chunks, nil
}

// setNestedDictBounded 限制重复路径产生的总分块体积，避免小输入因长键重复而放大为无界输出。
func setNestedDictBounded(d map[string]interface{}, path []string, value interface{}, budget *int) error {
	before := jsonSize(d)
	setNestedDict(d, path, value)
	*budget += jsonSize(d) - before
	if *budget > maxTextBytes {
		return errors.New("JSON 转换后的正文不能超过 2 MiB")
	}
	return nil
}

// setNestedDict sets a value in a nested dict structure, creating
// intermediate dicts as needed. This preserves the full JSON path.
//
// Example: setNestedDict(d, ["config","db","host"], "localhost")
// produces: {"config": {"db": {"host": "localhost"}}}
func setNestedDict(d map[string]interface{}, path []string, value interface{}) {
	if len(path) == 0 {
		return
	}
	current := d
	for _, key := range path[:len(path)-1] {
		next, ok := current[key]
		if !ok {
			next = map[string]interface{}{}
			current[key] = next
		}
		if nextDict, ok := next.(map[string]interface{}); ok {
			current = nextDict
		} else {
			// Path conflict (existing value is not a dict) — overwrite
			newDict := map[string]interface{}{}
			current[key] = newDict
			current = newDict
		}
	}
	current[path[len(path)-1]] = value
}

// listToDictPreprocess recursively converts JSON arrays to index-keyed
// dicts so the splitter can treat everything uniformly.
//
// Example: ["a","b","c"] → {"0":"a", "1":"b", "2":"c"}
func listToDictPreprocess(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[k] = listToDictPreprocess(val)
		}
		return result
	case []interface{}:
		result := make(map[string]interface{}, len(v))
		for i, item := range v {
			result[fmt.Sprintf("%d", i)] = listToDictPreprocess(item)
		}
		return result
	default:
		return data
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// jsonSize returns the serialized JSON byte length of a value.
func jsonSize(v interface{}) int {
	b, err := marshalJSON(v, "")
	if err != nil {
		return maxTextBytes + 1
	}
	return len(b)
}

func limitedJSONBlock(v interface{}) (string, error) {
	// 优先沿用上游的可读格式；缩进超限时改紧凑编码，仍保留全部路径与值。
	b, err := marshalJSON(v, "  ")
	if err != nil || len(b)+12 > maxTextBytes {
		b, err = marshalJSON(v, "")
	}
	if err != nil || len(b)+12 > maxTextBytes {
		return "", errors.New("JSON 转换后的正文不能超过 2 MiB")
	}
	return wrapCodeBlock(string(b)), nil
}

// formatValue formats a JSON value with indentation.
func formatValue(v interface{}) string {
	b, err := marshalJSON(v, "  ")
	if err != nil {
		b, _ = marshalJSON(v, "")
	}
	return string(b)
}

// marshalJSON encodes v like json.Marshal, indented when indent is set, but
// leaves <, > and & as they are. json.Marshal writes them as \u003c, \u003e
// and \u0026, which is meant for embedding in HTML and only garbles the text
// a chunk is read and searched as.
func marshalJSON(v interface{}, indent string) ([]byte, error) {
	var buf boundedJSONBuffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indent)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// wrapCodeBlock wraps content in a fenced JSON code block.
func wrapCodeBlock(content string) string {
	return "```json\n" + content + "\n```"
}

// trimBOM removes a UTF-8 BOM prefix if present.
func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

// canSplitDict returns true if a dict can be meaningfully split.
// A dict with multiple keys can be split by distributing keys across chunks.
// A dict with a single key can still be split if its value is a splittable dict.
func canSplitDict(d map[string]interface{}) bool {
	// Single-key containers do not prevent splitting a deeper multi-key object.
	// Walk iteratively so deeply wrapped exports do not add recursion here.
	for len(d) == 1 {
		for _, value := range d {
			sub, ok := value.(map[string]interface{})
			if !ok {
				return false
			}
			d = sub
		}
	}
	return len(d) > 1
}

// sortedKeys returns the keys of a map in sorted order.
// When all keys are numeric strings (from array-to-dict conversion),
// sorts numerically so "2" comes before "10".
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	allNumeric := true
	for k := range m {
		keys = append(keys, k)
		if allNumeric {
			if _, err := strconv.Atoi(k); err != nil {
				allNumeric = false
			}
		}
	}
	if allNumeric {
		sort.Slice(keys, func(i, j int) bool {
			ni, _ := strconv.Atoi(keys[i])
			nj, _ := strconv.Atoi(keys[j])
			return ni < nj
		})
	} else {
		sort.Strings(keys)
	}
	return keys
}

// boundedJSONBuffer 约束缩进编码的临时分配；过大时 formatValue 可退回紧凑编码。
type boundedJSONBuffer struct{ bytes.Buffer }

func (b *boundedJSONBuffer) Write(p []byte) (int, error) {
	if len(p) > maxTextBytes-b.Len() {
		return 0, errors.New("JSON 编码超过正文上限")
	}
	return b.Buffer.Write(p)
}
