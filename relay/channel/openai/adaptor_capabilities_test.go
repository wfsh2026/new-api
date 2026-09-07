package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestAppliesGPT6AstraCapabilities(t *testing.T) {
	maxTokens := uint(64)
	temperature := 0.2
	topP := 0.8
	logProbs := true
	topLogProbs := 5
	request := &dto.GeneralOpenAIRequest{
		Model: "gpt-6-astra",
		Messages: []dto.Message{
			{Role: "system", Content: "follow the instructions"},
			{Role: "user", Content: "hello"},
		},
		MaxTokens:   &maxTokens,
		Temperature: &temperature,
		TopP:        &topP,
		LogProbs:    &logProbs,
		TopLogProbs: &topLogProbs,
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-6-astra",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-6-astra",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)
	require.NoError(t, err)
	got, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)

	assert.Nil(t, got.MaxTokens)
	require.NotNil(t, got.MaxCompletionTokens)
	assert.Equal(t, uint(64), *got.MaxCompletionTokens)
	assert.Nil(t, got.Temperature)
	assert.Nil(t, got.TopP)
	assert.Nil(t, got.LogProbs)
	assert.Nil(t, got.TopLogProbs)
	assert.Equal(t, "developer", got.Messages[0].Role)
}
