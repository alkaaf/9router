package providers

import "strings"

// CompatiblePrefixes lists the dynamic provider-ID prefixes that
// represent a user-defined compatible or custom-embedding node.
// They are pass-through — no alias lookup is performed.
var CompatiblePrefixes = []string{
	"openai-compatible-",
	"anthropic-compatible-",
	"custom-embedding-",
}

// IsCompatible reports whether the given provider ID identifies a
// dynamic compatible / custom node.
func IsCompatible(providerID string) bool {
	for _, p := range CompatiblePrefixes {
		if strings.HasPrefix(providerID, p) {
			return true
		}
	}
	return false
}

// AliasToID maps short provider aliases to their canonical provider ID.
// The same map is used as the canonical registry — canonical IDs are
// also valid keys, so an "alias" lookup is never wrong for a
// canonical ID.
//
// The table mirrors open-sse/services/model.js#providerAliasToID.
var AliasToID = map[string]string{
	"cc":                "claude",
	"cx":                "codex",
	"gc":                "gemini-cli",
	"qw":                "qwen",
	"if":                "iflow",
	"ag":                "antigravity",
	"gh":                "github",
	"kr":                "kiro",
	"cu":                "cursor",
	"kc":                "kilocode",
	"kmc":               "kimi-coding",
	"cl":                "cline",
	"oc":                "opencode",
	"ocg":               "opencode-go",
	"qd":                "qoder",
	"qoder":             "qoder",
	"el":                "elevenlabs",
	"openai":            "openai",
	"vercel":            "vercel-ai-gateway",
	"vercel-ai-gateway": "vercel-ai-gateway",
	"anthropic":         "anthropic",
	"gemini":            "gemini",
	"openrouter":        "openrouter",
	"glm":               "glm",
	"kimi":              "kimi",
	"minimax":           "minimax",
	"minimax-cn":        "minimax-cn",
	"ds":                "deepseek",
	"deepseek":          "deepseek",
	"cmc":               "commandcode",
	"commandcode":       "commandcode",
	"groq":              "groq",
	"xai":               "xai",
	"mistral":           "mistral",
	"pplx":              "perplexity",
	"perplexity":        "perplexity",
	"together":          "together",
	"fireworks":         "fireworks",
	"cerebras":          "cerebras",
	"cohere":            "cohere",
	"nvidia":            "nvidia",
	"nebius":            "nebius",
	"siliconflow":       "siliconflow",
	"hyp":               "hyperbolic",
	"hyperbolic":        "hyperbolic",
	"dg":                "deepgram",
	"deepgram":          "deepgram",
	"aai":               "assemblyai",
	"assemblyai":        "assemblyai",
	"nb":                "nanobanana",
	"nanobanana":        "nanobanana",
	"ch":                "chutes",
	"chutes":            "chutes",
	"ark":               "volcengine-ark",
	"volcengine-ark":    "volcengine-ark",
	"byteplus":          "byteplus",
	"bpm":               "byteplus",
	"vx":                "vertex",
	"vertex":            "vertex",
	"vxp":               "vertex-partner",
	"vertex-partner":    "vertex-partner",
	"gw":                "grok-web",
	"grok-web":          "grok-web",
	"pw":                "perplexity-web",
	"perplexity-web":    "perplexity-web",
	"mimo":              "xiaomi-mimo",
	"xiaomi-mimo":       "xiaomi-mimo",
	"xmtp":              "xiaomi-tokenplan",
	"xiaomi-tokenplan":  "xiaomi-tokenplan",
	"cf":                "cloudflare-ai",
	"cloudflare-ai":     "cloudflare-ai",
	"fal":               "fal-ai",
	"fal-ai":            "fal-ai",
	"stability":         "stability-ai",
	"stability-ai":      "stability-ai",
	"bfl":               "black-forest-labs",
	"black-forest-labs": "black-forest-labs",
	"recraft":           "recraft",
	"topaz":             "topaz",
	"runway":            "runwayml",
	"runwayml":          "runwayml",
	"jina":              "jina-ai",
	"jina-ai":           "jina-ai",
	"polly":             "aws-polly",
	"aws-polly":         "aws-polly",
	"agentrouter":       "agentrouter",
	"aimlapi":           "aimlapi",
	"aiml":              "aimlapi",
	"novita":            "novita",
	"modal":             "modal",
	"mdl":               "modal",
	"reka":              "reka",
	"nlpcloud":          "nlpcloud",
	"nlpc":              "nlpcloud",
	"bazaarlink":        "bazaarlink",
	"bzl":               "bazaarlink",
	"completions":       "completions",
	"cpl":               "completions",
	"enally":            "enally",
	"enly":              "enally",
	"freetheai":         "freetheai",
	"fta":               "freetheai",
	"llm7":              "llm7",
	"lepton":            "lepton",
	"kluster":           "kluster",
	"ai21":              "ai21",
	"inference-net":     "inference-net",
	"inet":              "inference-net",
	"predibase":         "predibase",
	"bytez":             "bytez",
	"morph":             "morph",
	"longcat":           "longcat",
	"lc":                "longcat",
	"puter":             "puter",
	"pu":                "puter",
	"uncloseai":         "uncloseai",
	"unc":               "uncloseai",
	"scaleway":          "scaleway",
	"scw":               "scaleway",
	"deepinfra":         "deepinfra",
	"sambanova":         "sambanova",
	"samba":             "sambanova",
	"nscale":            "nscale",
	"baseten":           "baseten",
	"publicai":          "publicai",
	"nous-research":     "nous-research",
	"nous":              "nous-research",
	"glhf":              "glhf",
	"bb":                "blackbox",
	"blackbox":          "blackbox",
	"ollama":            "ollama-local",
	"ollama-local":      "ollama-local",
}

// ResolveProviderID normalises an alias or canonical ID to its
// canonical provider ID.
func ResolveProviderID(aliasOrID string) string {
	if aliasOrID == "" {
		return ""
	}
	if IsCompatible(aliasOrID) {
		return aliasOrID
	}
	if id, ok := AliasToID[aliasOrID]; ok {
		return id
	}
	return aliasOrID
}

// IsKnownProvider reports whether the given provider ID is part of
// the static registry (or a dynamic compatible prefix).
func IsKnownProvider(providerID string) bool {
	if providerID == "" {
		return false
	}
	if IsCompatible(providerID) {
		return true
	}
	if _, ok := AliasToID[providerID]; ok {
		return true
	}
	// Walk the map once to see if any value matches.
	for _, v := range AliasToID {
		if v == providerID {
			return true
		}
	}
	return false
}
