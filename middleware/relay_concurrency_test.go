package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayConcurrencyLimitDisabled(t *testing.T) {
	oldLimit := constant.RelayConcurrencyLimit
	constant.RelayConcurrencyLimit = 0
	t.Cleanup(func() { constant.RelayConcurrencyLimit = oldLimit })

	router := gin.New()
	router.GET("/v1/responses", RelayConcurrencyLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/responses", nil))
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestRelayConcurrencyLimitRejectsAndReleases(t *testing.T) {
	oldLimit := constant.RelayConcurrencyLimit
	constant.RelayConcurrencyLimit = 1
	t.Cleanup(func() { constant.RelayConcurrencyLimit = oldLimit })

	entered := make(chan struct{})
	release := make(chan struct{})
	firstStatus := make(chan int, 1)
	var calls atomic.Int32

	router := gin.New()
	router.GET("/v1/responses", RelayConcurrencyLimit(), func(c *gin.Context) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		c.Status(http.StatusOK)
	})

	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/responses", nil))
		firstStatus <- response.Code
	}()
	<-entered

	limited := httptest.NewRecorder()
	router.ServeHTTP(limited, httptest.NewRequest(http.MethodGet, "/v1/responses", nil))
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	require.Equal(t, "5", limited.Header().Get("Retry-After"))
	require.Contains(t, limited.Body.String(), "relay_concurrency_limit_exceeded")

	close(release)
	require.Equal(t, http.StatusOK, <-firstStatus)

	afterRelease := httptest.NewRecorder()
	router.ServeHTTP(afterRelease, httptest.NewRequest(http.MethodGet, "/v1/responses", nil))
	require.Equal(t, http.StatusOK, afterRelease.Code)
}

func TestRelayConcurrencyLimitUsesClaudeErrorShape(t *testing.T) {
	oldLimit := constant.RelayConcurrencyLimit
	constant.RelayConcurrencyLimit = 1
	t.Cleanup(func() { constant.RelayConcurrencyLimit = oldLimit })

	entered := make(chan struct{})
	release := make(chan struct{})
	router := gin.New()
	router.GET("/v1/messages", RelayConcurrencyLimit(), func(c *gin.Context) {
		select {
		case <-entered:
		default:
			close(entered)
			<-release
		}
		c.Status(http.StatusOK)
	})

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/messages", nil))
	}()
	<-entered

	limited := httptest.NewRecorder()
	router.ServeHTTP(limited, httptest.NewRequest(http.MethodGet, "/v1/messages", nil))
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	require.Contains(t, limited.Body.String(), "too many concurrent relay requests")

	close(release)
	<-firstDone
}
