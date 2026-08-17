package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesRequestFromCompactionPreservesCodexV2Fields(t *testing.T) {
	compact := &dto.OpenAIResponsesCompactionRequest{
		Model:             "gpt-5.6-sol",
		Input:             json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Instructions:      json.RawMessage(`"system prompt"`),
		Tools:             json.RawMessage(`[{"type":"function","name":"lookup"}]`),
		ParallelToolCalls: json.RawMessage(`true`),
		Reasoning:         &dto.Reasoning{Effort: "high"},
		Text:              json.RawMessage(`{"verbosity":"low"}`),
	}

	request := responsesRequestFromCompaction(compact, true)
	require.NotNil(t, request)
	assert.Equal(t, compact.Model, request.Model)
	assert.JSONEq(t, string(compact.Input), string(request.Input))
	assert.JSONEq(t, string(compact.Tools), string(request.Tools))
	assert.Same(t, compact.Reasoning, request.Reasoning)
	assert.JSONEq(t, string(compact.Text), string(request.Text))
}

func TestResponsesRequestFromCompactionStripsCodexOnlyFieldsForOtherChannels(t *testing.T) {
	compact := &dto.OpenAIResponsesCompactionRequest{
		Model:     "gpt-5.4",
		Input:     json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Tools:     json.RawMessage(`[{"type":"function","name":"lookup"}]`),
		Reasoning: &dto.Reasoning{Effort: "high"},
		Text:      json.RawMessage(`{"verbosity":"low"}`),
	}

	request := responsesRequestFromCompaction(compact, false)
	require.NotNil(t, request)
	assert.Nil(t, request.Tools)
	assert.Nil(t, request.Reasoning)
	assert.Nil(t, request.Text)
}
