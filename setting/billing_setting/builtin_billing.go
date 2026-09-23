package billing_setting

// Built-in token prices use actual USD per million tokens. Keep new model
// defaults here instead of splitting them across the legacy ratio tables.
var builtinBillingExpr = map[string]string{
	// https://developers.openai.com/api/docs/pricing (2026-09-23).
	// Cache reads and writes are separate from ordinary input. Long-context
	// pricing applies to the full request once total input exceeds 272K tokens.
	"gpt-6-sol":  `(len <= 272000 ? tier("standard", p * 2 + cr * 0.2 + cc * 2.5 + c * 10) : tier("long_context", p * 4 + cr * 0.4 + cc * 5 + c * 15)) * (param("service_tier") in ["fast", "priority"] ? 2 : 1) * (param("service_tier") == "flex" ? 0.5 : 1)`,
	"gpt-6-luna": `(len <= 272000 ? tier("standard", p * 0.1 + cr * 0.01 + cc * 0.125 + c * 0.5) : tier("long_context", p * 0.2 + cr * 0.02 + cc * 0.25 + c * 0.75)) * (param("service_tier") in ["fast", "priority"] ? 2 : 1) * (param("service_tier") == "flex" ? 0.5 : 1)`,
	// https://developers.openai.com/api/docs/pricing (Standard, 2026-09-09).
	// The Images API reports image output in output_tokens, normalized to c.
	"gpt-image-2":            `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	"gpt-image-2.5-sunburst": `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	"gpt-image-2.5-flare":    `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
}
