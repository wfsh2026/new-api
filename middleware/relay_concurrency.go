package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const relayConcurrencyRetryAfterSeconds = 5

// RelayConcurrencyLimit bounds requests for the complete relay lifecycle,
// including long-running streams. A non-positive limit leaves it disabled.
func RelayConcurrencyLimit() gin.HandlerFunc {
	limit := constant.RelayConcurrencyLimit
	if limit <= 0 {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	slots := make(chan struct{}, limit)
	return func(c *gin.Context) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			c.Next()
		default:
			c.Header("Retry-After", strconv.Itoa(relayConcurrencyRetryAfterSeconds))
			err := types.NewErrorWithStatusCode(
				fmt.Errorf("too many concurrent relay requests (limit: %d)", limit),
				"relay_concurrency_limit_exceeded",
				http.StatusTooManyRequests,
			)
			if strings.HasPrefix(c.Request.URL.Path, "/v1/messages") {
				c.JSON(err.StatusCode, gin.H{"error": err.ToClaudeError()})
			} else {
				c.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})
			}
			c.Abort()
		}
	}
}
