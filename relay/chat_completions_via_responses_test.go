package relay

import (
	"io"
	"math"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsResponsesEventStreamContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "plain", contentType: "text/event-stream", want: true},
		{name: "mixed case with charset", contentType: "Text/Event-Stream; charset=utf-8", want: true},
		{name: "json", contentType: "application/json", want: false},
		{name: "empty", contentType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResponsesEventStreamContentType(tt.contentType))
		})
	}
}

func TestSniffResponsesEventStreamBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "codex event stream without content type",
			body: "event: response.created\ndata: {\"type\":\"response.created\"}\n\n",
			want: true,
		},
		{
			name: "data only event stream",
			body: "data: {\"type\":\"response.output_text.delta\"}\n\n",
			want: true,
		},
		{
			name: "event stream with BOM and whitespace",
			body: "\ufeff \r\nevent: response.created\n",
			want: true,
		},
		{name: "json object", body: "{\"id\":\"resp_123\"}", want: false},
		{name: "json array", body: "[1,2,3]", want: false},
		{name: "plain text error", body: "error from upstream", want: false},
		{name: "short partial prefix", body: "eve", want: false},
		{name: "empty body", body: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isEventStream, body := sniffResponsesEventStreamBody(io.NopCloser(strings.NewReader(tt.body)))
			require.NotNil(t, body)
			defer body.Close()

			assert.Equal(t, tt.want, isEventStream)
			gotBody, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, tt.body, string(gotBody))
		})
	}
}

func TestSniffResponsesEventStreamBodyWithNilBody(t *testing.T) {
	isEventStream, body := sniffResponsesEventStreamBody(nil)
	assert.False(t, isEventStream)
	assert.Nil(t, body)
}

func TestRecalcQuotaFromRatiosIgnoresInvalidMultipliers(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"duration": 3,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.True(t, ok)
	assert.Equal(t, 150, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}

func TestRecalcQuotaFromRatiosRejectsAllInvalidAdjustedRatios(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.False(t, ok)
	assert.Equal(t, 0, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}
