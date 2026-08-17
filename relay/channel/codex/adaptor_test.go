package codex

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCodexTestContext(method, path, contentType string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, nil)
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	return c, recorder
}

func codexTestRelayInfo(relayMode int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode: relayMode,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeCodex,
			ChannelBaseUrl: "https://chatgpt.com",
			ApiKey:         `{"access_token":"access-token","account_id":"account-id"}`,
		},
	}
}

func TestCodexModelListIncludesImageModelWithoutCompactVariants(t *testing.T) {
	assert.Contains(t, ModelList, ImageModel)
	for _, model := range ModelList {
		assert.NotContains(t, model, "-openai-compact")
	}
}

func TestGetRequestURL(t *testing.T) {
	tests := []struct {
		name      string
		relayMode int
		want      string
	}{
		{name: "image generation", relayMode: relayconstant.RelayModeImagesGenerations, want: "https://chatgpt.com/backend-api/codex/images/generations"},
		{name: "image edit", relayMode: relayconstant.RelayModeImagesEdits, want: "https://chatgpt.com/backend-api/codex/images/edits"},
		{name: "responses", relayMode: relayconstant.RelayModeResponses, want: "https://chatgpt.com/backend-api/codex/responses"},
		{name: "responses compact via V2", relayMode: relayconstant.RelayModeResponsesCompact, want: "https://chatgpt.com/backend-api/codex/responses"},
		{name: "alpha search", relayMode: relayconstant.RelayModeAlphaSearch, want: "https://chatgpt.com/backend-api/codex/alpha/search"},
	}

	adaptor := &Adaptor{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := adaptor.GetRequestURL(codexTestRelayInfo(tt.relayMode))
			require.NoError(t, err)
			assert.Equal(t, tt.want, url)
		})
	}
}

func TestConvertImageGenerationRequest(t *testing.T) {
	n := uint(1)
	request := dto.ImageRequest{
		Model:          ImageModel,
		Prompt:         "a red fox in a field",
		N:              &n,
		Size:           "auto",
		Quality:        "auto",
		ResponseFormat: "url",
		Background:     json.RawMessage(`"auto"`),
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(nil, codexTestRelayInfo(relayconstant.RelayModeImagesGenerations), request)
	require.NoError(t, err)

	body, err := json.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"gpt-image-2",
		"prompt":"a red fox in a field",
		"n":1,
		"size":"auto",
		"quality":"auto",
		"background":"auto"
	}`, string(body))
	assert.NotContains(t, string(body), "response_format")
}

func TestConvertImageEditRequest(t *testing.T) {
	c, _ := newCodexTestContext(http.MethodPost, "/v1/images/edits", "application/json")
	request := dto.ImageRequest{
		Model:  ImageModel,
		Prompt: "add a red hat",
		Images: json.RawMessage(`[{"image_url":"data:image/png;base64,Zm9v"}]`),
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, codexTestRelayInfo(relayconstant.RelayModeImagesEdits), request)
	require.NoError(t, err)

	body, err := json.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"images":[{"image_url":"data:image/png;base64,Zm9v"}],
		"model":"gpt-image-2",
		"prompt":"add a red hat"
	}`, string(body))
}

func TestConvertImageEditRejectsMultipart(t *testing.T) {
	c, _ := newCodexTestContext(http.MethodPost, "/v1/images/edits", "multipart/form-data; boundary=test")
	_, err := (&Adaptor{}).ConvertImageRequest(c, codexTestRelayInfo(relayconstant.RelayModeImagesEdits), dto.ImageRequest{Model: ImageModel})
	require.ErrorContains(t, err, "JSON images array")
}

func TestSetupRequestHeaderForImage(t *testing.T) {
	c, _ := newCodexTestContext(http.MethodPost, "/v1/images/generations", "application/json; charset=utf-8")
	c.Request.Header.Set("x-codex-image-turn-id", "turn-123")
	c.Request.Header.Set("originator", "codex_vscode")

	header := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(c, &header, codexTestRelayInfo(relayconstant.RelayModeImagesGenerations))
	require.NoError(t, err)

	assert.Equal(t, "Bearer access-token", header.Get("Authorization"))
	assert.Equal(t, "account-id", header.Get("chatgpt-account-id"))
	assert.Equal(t, "turn-123", header.Get("x-codex-image-turn-id"))
	assert.Equal(t, "codex_vscode", header.Get("originator"))
	assert.Equal(t, "application/json", header.Get("Content-Type"))
	assert.Equal(t, "application/json", header.Get("Accept"))
}

func TestDoRequestProxiesCodexImageGeneration(t *testing.T) {
	type capturedRequest struct {
		path   string
		header http.Header
		body   string
	}
	captured := make(chan capturedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- capturedRequest{path: r.URL.Path, header: r.Header.Clone(), body: string(body)}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"Zm9v"}]}`))
	}))
	defer upstream.Close()

	c, _ := newCodexTestContext(http.MethodPost, "/v1/images/generations", "application/json")
	c.Request.Header.Set("x-codex-image-turn-id", "turn-123")
	c.Request.Header.Set("originator", "codex_vscode")
	info := codexTestRelayInfo(relayconstant.RelayModeImagesGenerations)
	info.ChannelBaseUrl = upstream.URL

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:      ImageModel,
		Prompt:     "a red fox in a field",
		Background: json.RawMessage(`"auto"`),
		Quality:    "auto",
		Size:       "auto",
	})
	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)

	respAny, err := (&Adaptor{}).DoRequest(c, info, bytes.NewReader(body))
	require.NoError(t, err)
	resp := respAny.(*http.Response)
	_ = resp.Body.Close()

	request := <-captured
	assert.Equal(t, "/backend-api/codex/images/generations", request.path)
	assert.Equal(t, "Bearer access-token", request.header.Get("Authorization"))
	assert.Equal(t, "account-id", request.header.Get("chatgpt-account-id"))
	assert.Equal(t, "turn-123", request.header.Get("x-codex-image-turn-id"))
	assert.Equal(t, "codex_vscode", request.header.Get("originator"))
	assert.Equal(t, "application/json", request.header.Get("Content-Type"))
	assert.JSONEq(t, `{
		"model":"gpt-image-2",
		"prompt":"a red fox in a field",
		"background":"auto",
		"quality":"auto",
		"size":"auto"
	}`, request.body)
}

func TestDoResponseHandlesCodexImageJSON(t *testing.T) {
	c, recorder := newCodexTestContext(http.MethodPost, "/v1/images/generations", "application/json")
	responseBody := `{
		"created":1778832973,
		"data":[{"b64_json":"Zm9v"}],
		"usage":{"input_tokens":6,"output_tokens":10,"total_tokens":16}
	}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}

	usageAny, newAPIError := (&Adaptor{}).DoResponse(c, resp, codexTestRelayInfo(relayconstant.RelayModeImagesGenerations))
	require.Nil(t, newAPIError)
	usage, ok := usageAny.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 6, usage.PromptTokens)
	assert.Equal(t, 10, usage.CompletionTokens)
	assert.Equal(t, 16, usage.TotalTokens)
	assert.JSONEq(t, responseBody, recorder.Body.String())
}

// The Codex backend rejects these fields, so the adaptor clears them rather
// than forwarding what the client sent.
func TestConvertOpenAIResponsesRequestDropsPenalties(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCodex},
		RelayMode:   relayconstant.RelayModeResponses,
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model:            "gpt-5-codex",
		Input:            json.RawMessage(`"hello"`),
		MaxOutputTokens:  lo.ToPtr(uint(128)),
		Temperature:      lo.ToPtr(1.0),
		FrequencyPenalty: json.RawMessage(`1.5`),
		PresencePenalty:  json.RawMessage(`1.5`),
	})
	require.NoError(t, err)

	request, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	assert.Nil(t, request.MaxOutputTokens)
	assert.Nil(t, request.Temperature)
	assert.Nil(t, request.FrequencyPenalty)
	assert.Nil(t, request.PresencePenalty)
}

func TestConvertCodexCompactionRequestToResponsesV2(t *testing.T) {
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, codexTestRelayInfo(relayconstant.RelayModeResponsesCompact), dto.OpenAIResponsesRequest{
		Model:             "gpt-5.6-sol",
		Input:             json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Instructions:      json.RawMessage(`"system prompt"`),
		Tools:             json.RawMessage(`[{"type":"function","name":"lookup"}]`),
		ParallelToolCalls: json.RawMessage(`true`),
		Reasoning:         &dto.Reasoning{Effort: "high", Summary: "auto"},
		Text:              json.RawMessage(`{"verbosity":"low"}`),
	})
	require.NoError(t, err)

	request, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.NotNil(t, request.Stream)
	assert.True(t, *request.Stream)
	assert.JSONEq(t, `false`, string(request.Store))
	assert.JSONEq(t, `["reasoning.encrypted_content"]`, string(request.Include))
	assert.JSONEq(t, `[
		{"role":"user","content":"hello"},
		{"type":"compaction_trigger"}
	]`, string(request.Input))
	assert.JSONEq(t, `[{"type":"function","name":"lookup"}]`, string(request.Tools))
	assert.Equal(t, "high", request.Reasoning.Effort)
	assert.JSONEq(t, `{"verbosity":"low"}`, string(request.Text))
}

func TestConvertCodexCompactionRequestDoesNotDuplicateTrigger(t *testing.T) {
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, codexTestRelayInfo(relayconstant.RelayModeResponsesCompact), dto.OpenAIResponsesRequest{
		Model: "gpt-5.6-sol",
		Input: json.RawMessage(`[{"role":"user","content":"hello"},{"type":"compaction_trigger"}]`),
	})
	require.NoError(t, err)

	request := converted.(dto.OpenAIResponsesRequest)
	var items []json.RawMessage
	require.NoError(t, json.Unmarshal(request.Input, &items))
	assert.Len(t, items, 2)
}

func TestCodexCompactionV2ProxyAndAggregation(t *testing.T) {
	type capturedRequest struct {
		path   string
		header http.Header
		body   []byte
	}
	captured := make(chan capturedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- capturedRequest{path: r.URL.Path, header: r.Header.Clone(), body: body}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Codex-Turn-State", "turn-state-1")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"compaction\",\"id\":\"cmp_1\",\"encrypted_content\":\"opaque-summary\"}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":120,\"output_tokens\":8,\"total_tokens\":128,\"input_tokens_details\":{\"cached_tokens\":100}}}}\n\n")
	}))
	defer upstream.Close()

	c, recorder := newCodexTestContext(http.MethodPost, "/v1/responses/compact", "application/json")
	info := codexTestRelayInfo(relayconstant.RelayModeResponsesCompact)
	info.ChannelBaseUrl = upstream.URL
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
		Model: "gpt-5.6-sol",
		Input: json.RawMessage(`[{"role":"user","content":"hello"}]`),
	})
	require.NoError(t, err)
	requestBody, err := json.Marshal(converted)
	require.NoError(t, err)

	respAny, err := (&Adaptor{}).DoRequest(c, info, bytes.NewReader(requestBody))
	require.NoError(t, err)
	usageAny, newAPIError := (&Adaptor{}).DoResponse(c, respAny.(*http.Response), info)
	require.Nil(t, newAPIError)

	request := <-captured
	assert.Equal(t, "/backend-api/codex/responses", request.path)
	assert.Equal(t, "text/event-stream", request.header.Get("Accept"))
	assert.Equal(t, "Bearer access-token", request.header.Get("Authorization"))
	var upstreamBody struct {
		Input  []json.RawMessage `json:"input"`
		Stream bool              `json:"stream"`
		Store  bool              `json:"store"`
	}
	require.NoError(t, json.Unmarshal(request.body, &upstreamBody))
	assert.Len(t, upstreamBody.Input, 2)
	assert.True(t, upstreamBody.Stream)
	assert.False(t, upstreamBody.Store)
	assert.JSONEq(t, `{"type":"compaction_trigger"}`, string(upstreamBody.Input[1]))

	usage := usageAny.(*dto.Usage)
	assert.Equal(t, 120, usage.PromptTokens)
	assert.Equal(t, 8, usage.CompletionTokens)
	assert.Equal(t, 128, usage.TotalTokens)
	assert.Equal(t, 100, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "turn-state-1", recorder.Header().Get("X-Codex-Turn-State"))
	assert.JSONEq(t, `{"output":[{"type":"compaction","id":"cmp_1","encrypted_content":"opaque-summary"}]}`, recorder.Body.String())
}

func TestScanCodexCompactionStreamRejectsMissingCompletedEvent(t *testing.T) {
	_, err := scanCodexCompactionStream(strings.NewReader(
		"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\",\"encrypted_content\":\"opaque\"}}\n\n",
	))
	require.ErrorContains(t, err, "before response.completed")
}
