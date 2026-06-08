package system

import (
	"encoding/json"
	"net/http"
)

// ollamaModels is the canonical list of Ollama model tags. It is
// defined as a package-level variable so that build-time injection
// can override it via ldflags when needed.
var ollamaModels = []string{
	"llama2", "llama2:13b", "codellama:7b", "mistral",
	"mixtral", "phi:2", "gemma:2b", "gemma:7b",
	"qwen2:7b", "deepseek-coder:6.7b",
}

// TagsHandler returns an http.HandlerFunc for GET /api/tags.
// Returns the static list of Ollama model tags as a JSON array.
func TagsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ollamaModels)
	}
}
