package billing_setting

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPT6SolAndLunaBuiltinBilling(t *testing.T) {
	models := []struct {
		name  string
		scale float64
	}{
		{name: "gpt-6-sol", scale: 1},
		{name: "gpt-6-luna", scale: 0.05},
	}
	cases := []struct {
		name   string
		length float64
		body   string
		tier   string
		cost   float64
	}{
		{name: "standard boundary", length: 272000, body: `{}`, tier: "standard", cost: 57850},
		{name: "long context boundary", length: 272001, body: `{}`, tier: "long_context", cost: 115450.4},
		{name: "fast", length: 272000, body: `{"service_tier":"fast"}`, tier: "standard", cost: 115700},
		{name: "priority alias", length: 272000, body: `{"service_tier":"priority"}`, tier: "standard", cost: 115700},
		{name: "flex long context", length: 272001, body: `{"service_tier":"flex"}`, tier: "long_context", cost: 57725.2},
	}
	for _, model := range models {
		expression := builtinBillingExpr[model.name]
		for _, tc := range cases {
			testName := model.name + "/" + tc.name
			t.Run(testName, func(t *testing.T) {
				// Most input is cached: pricing must use total context length,
				// not the much smaller number of ordinary input tokens.
				params := billingexpr.TokenParams{P: 1000, CR: tc.length - 1500, CC: 500, C: 50, Len: tc.length}
				body := []byte(tc.body)
				request := billingexpr.RequestInput{Body: body}
				cost, trace, err := billingexpr.RunExprWithRequest(expression, params, request)
				require.NoError(t, err)
				expected := tc.cost * model.scale
				difference := math.Abs(cost - expected)
				assert.LessOrEqual(t, difference, 1e-8)
				assert.Equal(t, tc.tier, trace.MatchedTier)
			})
		}
	}
}
