package studio

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/fpokrzywa/AppRivet/internal/credentials"
)

func savedProviders(vault credentials.Vault) []string {
	saved := []string{}
	if vault != nil {
		for _, provider := range []string{"openai", "anthropic"} {
			if key, err := vault.Get(provider); err == nil && credentials.ValidKey(key) {
				saved = append(saved, provider)
			}
		}
	}
	return saved
}

func registerCredentialHandlers(mux *http.ServeMux, vault credentials.Vault) {
	mux.HandleFunc("POST /api/provider-key", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Provider string `json:"provider"`
			Key      string `json:"key"`
		}
		if !decodeCredentialRequest(w, r, &req) || !credentials.ValidProvider(req.Provider) {
			writeError(w, http.StatusBadRequest, "Enter a valid API key for OpenAI or Anthropic.")
			return
		}
		key := strings.TrimSpace(req.Key)
		if !credentials.ValidKey(key) {
			writeError(w, http.StatusBadRequest, "Enter a valid API key for OpenAI or Anthropic.")
			return
		}
		if vault == nil || vault.Save(req.Provider, key) != nil {
			writeError(w, http.StatusServiceUnavailable, "Could not save this key in your system credential store.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/remove-provider-key", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Provider string `json:"provider"`
		}
		if !decodeCredentialRequest(w, r, &req) || !credentials.ValidProvider(req.Provider) {
			writeError(w, http.StatusBadRequest, "Choose OpenAI or Anthropic to remove the saved key.")
			return
		}
		if vault == nil || vault.Remove(req.Provider) != nil {
			writeError(w, http.StatusServiceUnavailable, "Could not remove this key from your system credential store.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func decodeCredentialRequest(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst) == nil && decoder.Decode(new(any)) == io.EOF
}

func generationKey(w http.ResponseWriter, vault credentials.Vault, provider, key string) (string, bool) {
	if key != "" || vault == nil {
		return key, true
	}
	key, err := vault.Get(provider)
	if err != nil || (key != "" && !credentials.ValidKey(key)) {
		writeError(w, http.StatusServiceUnavailable, "Could not load the saved API key. Unlock your system credential store or enter a key for this session.")
		return "", false
	}
	return key, true
}

func approveComponentCall(w http.ResponseWriter, provided, expected string) bool {
	if provided == expected {
		return true
	}
	writeError(w, http.StatusBadRequest, "Review and approve the current prompts, provider, model and token budget before sending.")
	return false
}

func writeGenerationError(w http.ResponseWriter, err error, key string) {
	message := err.Error()
	if key = strings.TrimSpace(key); credentials.ValidKey(key) {
		message = strings.ReplaceAll(message, key, "[redacted]")
	}
	writeError(w, http.StatusBadRequest, message)
}
