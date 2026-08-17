package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelRequestResponsesCompactPreservesModelName(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/responses/compact",
		bytes.NewBufferString(`{"model":"gpt-5.6-sol","input":"compact this conversation"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)

	request, shouldSelectChannel, err := getModelRequest(c)
	require.NoError(t, err)
	require.NotNil(t, request)
	assert.True(t, shouldSelectChannel)
	assert.Equal(t, "gpt-5.6-sol", request.Model)
}
