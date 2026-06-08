package executor

import (
	"net/http"
	"strings"
)

// DefaultAzureAPIVersion is the API version used when the config omits
// one. The Node.js implementation uses "2024-02-15-preview".
const DefaultAzureAPIVersion = "2024-02-15-preview"

// AzureExecutor talks to Azure OpenAI Service. Unlike the plain OpenAI
// API, Azure requires:
//
//   - The deployment name embedded in the path rather than the model
//     query parameter.
//   - An api-key header, not a Bearer token.
//   - An api-version query parameter.
//   - An optional OpenAI-Organization header.
type AzureExecutor struct {
	*BaseExecutor

	endpoint     string
	apiVersion   string
	deployment   string
	organization string
}

// NewAzureExecutor returns an AzureExecutor wired to the supplied
// endpoint, deployment, API version, and optional org. Empty
// apiVersion is replaced with DefaultAzureAPIVersion.
func NewAzureExecutor(endpoint, apiVersion, deployment, organization string) *AzureExecutor {
	if apiVersion == "" {
		apiVersion = DefaultAzureAPIVersion
	}
	cfg := &ProviderConfig{
		Provider:   "azure",
		BaseURLs:   []string{endpoint},
		AuthHeader: "api-key",
		MaxRetries: DefaultMaxRetries,
	}
	return &AzureExecutor{
		BaseExecutor: NewBaseExecutor(cfg, nil),
		endpoint:     endpoint,
		apiVersion:   apiVersion,
		deployment:   deployment,
		organization: organization,
	}
}

// NewDefaultAzureExecutor returns an AzureExecutor with safe defaults.
func NewDefaultAzureExecutor() *AzureExecutor {
	return NewAzureExecutor("", DefaultAzureAPIVersion, "", "")
}

// BuildUrl returns the Azure chat-completions URL. The deployment name
// is embedded in the path; api-version is a query parameter; stream
// is appended when enabled.
func (a *AzureExecutor) BuildUrl(model string, stream bool, urlIndex int) string {
	endpoint := a.endpoint
	deployment := a.deployment
	if deployment == "" {
		deployment = model
	}
	path := "/openai/deployments/" + deployment + "/chat/completions"
	u := joinURL(endpoint, path)
	if a.apiVersion != "" {
		if strings.Contains(u, "?") {
			u += "&api-version=" + a.apiVersion
		} else {
			u += "?api-version=" + a.apiVersion
		}
	}
	if stream {
		sep := "&"
		if !strings.Contains(u, "?") {
			sep = "?"
		}
		u += sep + "stream=true"
	}
	return u
}

// BuildHeaders adds the api-key header (from the BaseExecutor config)
// and the optional OpenAI-Organization header.
func (a *AzureExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := a.BaseExecutor.BuildHeaders(req, creds)
	if a.organization != "" {
		h.Set("OpenAI-Organization", a.organization)
	}
	return h
}

// TransformRequest is a passthrough — Azure uses the same wire format
// as OpenAI, so the body the translator produces is already valid.
func (a *AzureExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	return body, nil
}

// ────────────────────────────────────────────────────────────────────
// AzureConfig — typed config carrier used by the chat handler
// ────────────────────────────────────────────────────────────────────

// AzureConfig is the typed configuration for an Azure executor instance.
// The chat handler constructs this from providerSpecificData on the
// connection entry and calls ApplyToExecutor to mutate an executor in
// place before each request.
type AzureConfig struct {
	Endpoint     string `json:"endpoint"`
	APIVersion   string `json:"apiVersion"`
	Deployment   string `json:"deployment"`
	Organization string `json:"organization,omitempty"`
}

// ApplyToExecutor mutates the supplied AzureExecutor to use the config's
// parameters. Only non-empty fields are applied — this lets callers
// send a partial config without clobbering values that were populated
// previously.
func (c AzureConfig) ApplyToExecutor(e *AzureExecutor) {
	if c.Endpoint != "" {
		e.endpoint = c.Endpoint
	}
	if c.APIVersion != "" {
		e.apiVersion = c.APIVersion
	}
	if c.Deployment != "" {
		e.deployment = c.Deployment
	}
	if c.Organization != "" {
		e.organization = c.Organization
	}
}

func init() {
	Register("azure", func() Executor { return NewDefaultAzureExecutor() })
}

// compiled check.
var _ = http.Header{}
