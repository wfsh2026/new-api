package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

var compactionTrigger = json.RawMessage(`{"type":"compaction_trigger"}`)

func appendCompactionTrigger(input json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(input))
	if trimmed == "" || trimmed == "null" {
		return nil, errors.New("codex compact: input is required")
	}

	var items []json.RawMessage
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(input, &items); err != nil {
			return nil, fmt.Errorf("codex compact: invalid input array: %w", err)
		}
	case '"':
		if !json.Valid(input) {
			return nil, errors.New("codex compact: invalid input string")
		}
		items = []json.RawMessage{json.RawMessage(`{"role":"user","content":` + trimmed + `}`)}
	default:
		return nil, errors.New("codex compact: input must be a string or an array")
	}

	if len(items) > 0 {
		var last struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(items[len(items)-1], &last) == nil && last.Type == "compaction_trigger" {
			return json.Marshal(items)
		}
	}

	items = append(items, compactionTrigger)
	return json.Marshal(items)
}

type codexCompactionStreamEvent struct {
	Type     string                       `json:"type"`
	Item     json.RawMessage              `json:"item"`
	Response *dto.OpenAIResponsesResponse `json:"response,omitempty"`
	Error    any                          `json:"error,omitempty"`
	Message  string                       `json:"message,omitempty"`
}

type codexCompactionScanResult struct {
	output []json.RawMessage
	usage  *dto.Usage
}

func scanCodexCompactionStream(reader io.Reader) (codexCompactionScanResult, error) {
	result := codexCompactionScanResult{}
	scanner := helper.NewStreamScanner(reader)
	sawCompleted := false
	outputItemCount := 0

scanLoop:
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var event codexCompactionStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return result, fmt.Errorf("codex compact: invalid SSE event: %w", err)
		}

		switch event.Type {
		case dto.ResponsesOutputTypeItemDone:
			outputItemCount++
			var item struct {
				Type             string `json:"type"`
				EncryptedContent string `json:"encrypted_content"`
			}
			if err := json.Unmarshal(event.Item, &item); err != nil {
				return result, fmt.Errorf("codex compact: invalid output item: %w", err)
			}
			if item.Type == "compaction" {
				if item.EncryptedContent == "" {
					return result, errors.New("codex compact: compaction output has no encrypted_content")
				}
				result.output = append(result.output, append(json.RawMessage(nil), event.Item...))
			}
		case "response.completed", "response.done":
			if event.Response == nil {
				return result, errors.New("codex compact: completed event has no response")
			}
			if upstreamError := event.Response.GetOpenAIError(); upstreamError != nil && upstreamError.Message != "" {
				return result, fmt.Errorf("codex compact: upstream error: %s", upstreamError.Message)
			}
			sawCompleted = true
			result.usage = event.Response.Usage
			break scanLoop
		case "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			message := "upstream compaction failed"
			if event.Response != nil {
				if upstreamError := event.Response.GetOpenAIError(); upstreamError != nil && upstreamError.Message != "" {
					message = upstreamError.Message
				}
			}
			return result, fmt.Errorf("codex compact: %s", message)
		case "error":
			message := event.Message
			if upstreamError := dto.GetOpenAIError(event.Error); upstreamError != nil && upstreamError.Message != "" {
				message = upstreamError.Message
			}
			if message == "" {
				message = "upstream returned an error event"
			}
			return result, fmt.Errorf("codex compact: %s", message)
		}
	}

	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("codex compact: read SSE stream: %w", err)
	}
	if !sawCompleted {
		return result, errors.New("codex compact: stream closed before response.completed")
	}
	if len(result.output) != 1 {
		return result, fmt.Errorf("codex compact: expected exactly one compaction output item, got %d from %d output items", len(result.output), outputItemCount)
	}
	return result, nil
}

func codexResponsesCompactionV2Handler(c *gin.Context, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("codex compact: invalid response"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	defer service.CloseResponseBodyGracefully(resp)

	resultCh := make(chan struct {
		result codexCompactionScanResult
		err    error
	}, 1)
	go func() {
		result, err := scanCodexCompactionStream(resp.Body)
		resultCh <- struct {
			result codexCompactionScanResult
			err    error
		}{result: result, err: err}
	}()

	timeout := time.Duration(constant.StreamingTimeout) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var scan struct {
		result codexCompactionScanResult
		err    error
	}
	select {
	case scan = <-resultCh:
	case <-c.Request.Context().Done():
		_ = resp.Body.Close()
		<-resultCh
		return nil, types.NewError(c.Request.Context().Err(), types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry(), types.ErrOptionWithStatusCode(http.StatusRequestTimeout))
	case <-timer.C:
		_ = resp.Body.Close()
		<-resultCh
		return nil, types.NewError(errors.New("codex compact: upstream stream timed out"), types.ErrorCodeChannelResponseTimeExceeded, types.ErrOptionWithStatusCode(http.StatusGatewayTimeout))
	}
	if scan.err != nil {
		return nil, types.NewOpenAIError(scan.err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}

	responseBody, err := json.Marshal(struct {
		Output []json.RawMessage `json:"output"`
	}{Output: scan.result.output})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	resp.Header.Set("Content-Type", "application/json")
	service.IOCopyBytesGracefully(c, resp, responseBody)

	usage := &dto.Usage{}
	if scan.result.usage != nil {
		usage.PromptTokens = scan.result.usage.InputTokens
		usage.CompletionTokens = scan.result.usage.OutputTokens
		usage.TotalTokens = scan.result.usage.TotalTokens
		if scan.result.usage.InputTokensDetails != nil {
			usage.PromptTokensDetails = *scan.result.usage.InputTokensDetails
		}
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage, nil
}
