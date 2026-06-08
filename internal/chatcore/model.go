package chatcore

import (
	"strings"
)

// ModelInfo is the resolved chat model reference. It mirrors the
// shape produced by open-sse/services/model.js#getModelInfoCore — the
// fields the downstream routing layer reads (provider, model) are
// preserved verbatim, and the diagnostic flags (IsAlias, ProviderAlias)
// are kept for logging.
type ModelInfo struct {
	// Provider is the canonical provider id (e.g. "openai",
	// "anthropic"). May be empty when the input is an alias that
	// could not be resolved to any known provider — in that case
	// the request must be rejected downstream.
	Provider string

	// Model is the provider-specific model name (e.g. "gpt-4",
	// "claude-3-5-sonnet-20241022"). It is the portion after the
	// first "/" in "provider/model", or the full input when no
	// "/" is present (alias form).
	Model string

	// IsAlias is true when the input had no "/" separator and was
	// treated as an alias lookup or a fallback inference.
	IsAlias bool

	// ProviderAlias is the literal text that appeared before the
	// first "/" in the input. It is kept so the log line can show
	// "cc/claude-3" was expanded to "anthropic/claude-3". It is
	// empty when the input had no "/" separator.
	ProviderAlias string
}

// parseModel implements the first half of the Node.js
// open-sse/services/model.js#parseModel helper. It splits the input
// on the FIRST "/" and resolves the leading segment against the
// provider-alias table. Multiple slashes are legal — the model
// portion can itself contain slashes (rare but valid in some
// provider model ids).
//
//   "openai/gpt-4"        → provider="openai",   model="gpt-4", isAlias=false
//   "cc/claude-3"         → provider="anthropic", model="claude-3", isAlias=false, providerAlias="cc"
//   "gpt-4"               → provider="",          model="gpt-4", isAlias=true
//   "openrouter/owner/repo" → provider="openrouter", model="owner/repo"
//
// An empty input returns a zero ModelInfo with IsAlias=false.
func parseModel(modelStr string) ModelInfo {
	if modelStr == "" {
		return ModelInfo{}
	}

	if i := strings.Index(modelStr, "/"); i >= 0 {
		providerOrAlias := modelStr[:i]
		model := modelStr[i+1:]
		return ModelInfo{
			Provider:      resolveProviderAlias(providerOrAlias),
			Model:         model,
			IsAlias:       false,
			ProviderAlias: providerOrAlias,
		}
	}

	return ModelInfo{
		Provider: "",
		Model:    modelStr,
		IsAlias:  true,
	}
}

// resolveProviderAlias maps a short alias to its canonical provider
// id, mirroring the ALIAS_TO_PROVIDER_ID map in
// open-sse/services/model.js. Unknown inputs are returned unchanged so
// the alias and canonical id are interchangeable on the wire.
//
// The table is intentionally hard-coded — it has been stable in the
// Node.js codebase for a long time, and the values are needed at
// every chat request (no DB hit). If a new provider is added in the
// future, the table here and the JS one must be updated in lockstep.
var providerAliasToID = map[string]string{
	"cc":              "claude",
	"cx":              "codex",
	"gc":              "gemini-cli",
	"qw":              "qwen",
	"if":              "iflow",
	"ag":              "antigravity",
	"gh":              "github",
	"kr":              "kiro",
	"cu":              "cursor",
	"kc":              "kilocode",
	"kmc":             "kimi-coding",
	"cl":              "cline",
	"oc":              "opencode",
	"ocg":             "opencode-go",
	"qd":              "qoder",
	"qoder":           "qoder",
	"el":              "elevenlabs",
	"openai":          "openai",
	"vercel":          "vercel-ai-gateway",
	"vercel-ai-gateway": "vercel-ai-gateway",
	"anthropic":       "anthropic",
	"gemini":          "gemini",
	"openrouter":      "openrouter",
	"glm":             "glm",
	"kimi":            "kimi",
	"minimax":         "minimax",
	"minimax-cn":      "minimax-cn",
	"ds":              "deepseek",
	"deepseek":        "deepseek",
	"cmc":             "commandcode",
	"commandcode":     "commandcode",
	"groq":            "groq",
	"xai":             "xai",
	"mistral":         "mistral",
	"pplx":            "perplexity",
	"perplexity":      "perplexity",
	"together":        "together",
	"fireworks":       "fireworks",
	"cerebras":        "cerebras",
	"cohere":          "cohere",
	"nvidia":          "nvidia",
	"nebius":          "nebius",
	"siliconflow":     "siliconflow",
	"hyp":             "hyperbolic",
	"hyperbolic":      "hyperbolic",
	"dg":              "deepgram",
	"deepgram":        "deepgram",
	"aai":             "assemblyai",
	"assemblyai":      "assemblyai",
	"nb":              "nanobanana",
	"nanobanana":      "nanobanana",
	"ch":              "chutes",
	"chutes":          "chutes",
	"ark":             "volcengine-ark",
	"volcengine-ark":  "volcengine-ark",
	"byteplus":        "byteplus",
	"bpm":             "byteplus",
	"vx":              "vertex",
	"vertex":          "vertex",
	"vxp":             "vertex-partner",
	"vertex-partner":  "vertex-partner",
	"gw":              "grok-web",
	"grok-web":        "grok-web",
	"pw":              "perplexity-web",
	"perplexity-web":  "perplexity-web",
	"mimo":            "xiaomi-mimo",
	"xiaomi-mimo":     "xiaomi-mimo",
	"xmtp":            "xiaomi-tokenplan",
	"xiaomi-tokenplan": "xiaomi-tokenplan",
	"cf":              "cloudflare-ai",
	"cloudflare-ai":   "cloudflare-ai",
	"fal":             "fal-ai",
	"fal-ai":          "fal-ai",
	"stability":       "stability-ai",
	"stability-ai":    "stability-ai",
	"bfl":             "black-forest-labs",
	"black-forest-labs": "black-forest-labs",
	"recraft":         "recraft",
	"topaz":           "topaz",
	"runway":          "runwayml",
	"runwayml":        "runwayml",
	"jina":            "jina-ai",
	"jina-ai":         "jina-ai",
	"polly":           "aws-polly",
	"aws-polly":       "aws-polly",
	"agentrouter":     "agentrouter",
	"aimlapi":         "aimlapi",
	"aiml":            "aimlapi",
	"novita":          "novita",
	"modal":           "modal",
	"mdl":             "modal",
	"reka":            "reka",
	"nlpcloud":        "nlpcloud",
	"nlpc":            "nlpcloud",
	"bazaarlink":      "bazaarlink",
	"bzl":             "bazaarlink",
	"completions":     "completions",
	"cpl":             "completions",
	"enally":          "enally",
	"enly":            "enally",
	"freetheai":       "freetheai",
	"fta":             "freetheai",
	"llm7":            "llm7",
	"lepton":          "lepton",
	"kluster":         "kluster",
	"ai21":            "ai21",
	"inference-net":   "inference-net",
	"inet":            "inference-net",
	"predibase":       "predibase",
	"bytez":           "bytez",
	"morph":           "morph",
	"longcat":         "longcat",
	"lc":              "longcat",
	"puter":           "puter",
	"pu":              "puter",
	"uncloseai":       "uncloseai",
	"unc":             "uncloseai",
	"scaleway":        "scaleway",
	"scw":             "scaleway",
	"deepinfra":       "deepinfra",
	"sambanova":       "sambanova",
	"samba":           "sambanova",
	"nscale":          "nscale",
	"baseten":         "baseten",
	"publicai":        "publicai",
	"nous-research":   "nous-research",
	"nous":            "nous-research",
	"glhf":            "glhf",
	"bb":              "blackbox",
	"blackbox":        "blackbox",
}

func resolveProviderAlias(aliasOrID string) string {
	if id, ok := providerAliasToID[aliasOrID]; ok {
		return id
	}
	return aliasOrID
}

// resolveModelAlias looks up a model alias (key in the user-defined
// aliases map) and returns the canonical provider/model. The aliases
// map mirrors the per-user modelAliases setting; it is supplied by
// the caller (typically populated from the settings row).
//
// The map values can be either "provider/model" strings or objects
// with { provider, model } fields — the JS implementation supports
// both shapes. In Go we accept only the string form for the in-memory
// representation; the caller is responsible for any JSON
// normalisation before populating the map.
func resolveModelAlias(alias string, aliases map[string]string) (provider, model string, ok bool) {
	v, present := aliases[alias]
	if !present {
		return "", "", false
	}
	if i := strings.Index(v, "/"); i >= 0 {
		return resolveProviderAlias(v[:i]), v[i+1:], true
	}
	// No "/" — treat the whole value as a model name with an empty
	// provider. Downstream code will fall back to inference.
	return "", v, true
}

// inferProviderFromModelName implements the Node.js
// inferProviderFromModelName helper. It is used as a last-resort
// fallback when the input has no "/" separator and no matching alias
// is found — the Node.js code does the same.
func inferProviderFromModelName(modelName string) string {
	m := strings.ToLower(modelName)
	switch {
	case strings.HasPrefix(m, "claude-"):
		return "anthropic"
	case strings.HasPrefix(m, "gemini-"):
		return "gemini"
	case strings.HasPrefix(m, "gpt-"):
		return "openai"
	case strings.HasPrefix(m, "o1"), strings.HasPrefix(m, "o3"), strings.HasPrefix(m, "o4"):
		return "openai"
	case strings.HasPrefix(m, "deepseek-"):
		return "openrouter"
	}
	return "openai"
}

// ResolveModel is the top-level entry point. It mirrors
// open-sse/services/model.js#getModelInfoCore exactly:
//
//  1. Parse the input via parseModel.
//  2. If the parsed result is NOT an alias (i.e. had a "/"), return
//     it as-is — the provider segment has already been resolved.
//  3. Otherwise, look up the alias in the supplied map. If found,
//     return the resolved provider/model.
//  4. Otherwise, fall back to inferProviderFromModelName.
//
// The aliases map may be nil; in that case step 3 is skipped and
// the function goes straight to step 4.
func ResolveModel(modelStr string, aliases map[string]string) ModelInfo {
	parsed := parseModel(modelStr)
	if !parsed.IsAlias {
		// Explicit "provider/model" form. Provider is already
		// resolved; we do not consult the alias map.
		return parsed
	}
	// Alias form. Try the user-defined aliases first, then fall back
	// to heuristic inference.
	if p, m, ok := resolveModelAlias(parsed.Model, aliases); ok {
		return ModelInfo{Provider: p, Model: m, IsAlias: true}
	}
	return ModelInfo{Provider: inferProviderFromModelName(parsed.Model), Model: parsed.Model, IsAlias: true}
}
