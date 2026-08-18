package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDistributeReturnsRequestEntityTooLarge(t *testing.T) {
	require.NoError(t, appI18n.Init())

	oldLimit := constant.MaxRequestBodyMB
	oldDiskCache := common.GetDiskCacheConfig()
	constant.MaxRequestBodyMB = 1
	common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: false})
	t.Cleanup(func() {
		constant.MaxRequestBodyMB = oldLimit
		common.SetDiskCacheConfig(oldDiskCache)
	})

	router := gin.New()
	router.POST("/v1/responses", Distribute(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	body := strings.NewReader(strings.Repeat("a", (1<<20)+1))
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.Contains(t, response.Body.String(), "request body exceeds 1 MB")
}
