package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go-server/internal/app/chat/port"
	domain "go-server/internal/domain/ai"
	"go-server/internal/infra/ai/providers"
)

// Stream 串行发送正文增量；没有结束标记的断流必须报告失败。
func (c *protocolDriver) Stream(ctx context.Context, definition providers.Definition, model domain.Model, in port.ChatRequest, emit func(string) error) error {
	if model.Kind != "chat" || strings.TrimSpace(model.ModelName) == "" {
		return errors.New("对话模型配置无效")
	}
	protocol := c.protocol
	if protocol == "" {
		return errors.New("对话协议不受支持")
	}
	address, err := protocolEndpoint(model.Endpoint, definition.Compatibility.RootPath, chatPath(protocol))
	if err != nil {
		return err
	}
	messages := make([]map[string]string, 0, len(in.Messages))
	for _, m := range in.Messages {
		messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
	}
	payload := map[string]any{"model": model.ModelName, "messages": messages, "stream": true}
	if protocol == "ollama" {
		payload["options"] = map[string]any{"temperature": in.Temperature, "num_predict": in.MaxTokens}
	} else {
		if !definition.Compatibility.OmitTemperature {
			payload["temperature"] = in.Temperature
		}
		payload[definition.Compatibility.TokenField] = in.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.New("无法编码对话模型请求")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return errors.New("无法创建对话模型请求")
	}
	req.Header.Set("Content-Type", "application/json")
	if protocol == "openai" {
		req.Header.Set("Accept", "text/event-stream")
	}
	if model.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+model.APIKey)
	}
	response, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("无法连接对话模型服务")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("对话模型返回 HTTP %d", response.StatusCode)
	}
	reader := bufio.NewScanner(io.LimitReader(response.Body, 8*1024*1024+1))
	reader.Buffer(make([]byte, 4096), 1024*1024)
	finished := false
	total := 0
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		total += len(line)
		if total > 8*1024*1024 {
			return errors.New("对话模型响应超过限制")
		}
		if line == "" {
			continue
		}
		if protocol == "openai" {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				finished = true
				break
			}
			var event struct {
				Error   json.RawMessage `json:"error"`
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err = json.Unmarshal([]byte(data), &event); err != nil {
				return errors.New("对话模型返回无效流式数据")
			}
			if len(event.Error) > 0 && string(event.Error) != "null" {
				return errors.New("对话模型返回生成错误")
			}
			if len(event.Choices) > 0 {
				choice := event.Choices[0]
				if choice.Delta.Content != "" {
					if err = emit(choice.Delta.Content); err != nil {
						return err
					}
				}
				if choice.FinishReason != nil && *choice.FinishReason != "" {
					finished = true
				}
			}
		} else {
			var event struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Done  bool            `json:"done"`
				Error json.RawMessage `json:"error"`
			}
			if err = json.Unmarshal([]byte(line), &event); err != nil {
				return errors.New("Ollama 返回无效流式数据")
			}
			if len(event.Error) > 0 && string(event.Error) != "null" {
				return errors.New("Ollama 返回生成错误")
			}
			if event.Message.Content != "" {
				if err = emit(event.Message.Content); err != nil {
					return err
				}
			}
			if event.Done {
				finished = true
				break
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err = reader.Err(); err != nil {
		return errors.New("读取对话模型响应失败")
	}
	if !finished {
		return errors.New("对话模型响应中断，未收到完成标记")
	}
	return nil
}
