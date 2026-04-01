package tui

// AIConfig holds the configuration for AI provider access.
// Embedded by all AI-consuming components to avoid field duplication.
type AIConfig struct {
	Provider      string
	Model         string
	OllamaURL     string
	APIKey        string
	NousURL       string
	NousAPIKey    string
	NerveBinary   string
	NerveModel    string
	NerveProvider string
}

// SetFromConfig populates the AIConfig from the app's config values.
func (c *AIConfig) SetFromConfig(provider, model, ollamaURL, apiKey, nousURL, nousAPIKey, nerveBinary, nerveModel, nerveProvider string) {
	c.Provider = provider
	c.Model = model
	c.OllamaURL = ollamaURL
	c.APIKey = apiKey
	c.NousURL = nousURL
	c.NousAPIKey = nousAPIKey
	c.NerveBinary = nerveBinary
	c.NerveModel = nerveModel
	c.NerveProvider = nerveProvider
}

// NewNerve creates a NerveClient from this config.
func (c AIConfig) NewNerve() *NerveClient {
	return NewNerveClient(c.NerveBinary, c.NerveModel, c.NerveProvider)
}

// NewNous creates a NousClient from this config.
func (c AIConfig) NewNous() *NousClient {
	return NewNousClient(c.NousURL, c.NousAPIKey)
}

// OllamaEndpoint returns the Ollama URL with a default fallback.
func (c AIConfig) OllamaEndpoint() string {
	if c.OllamaURL != "" {
		return c.OllamaURL
	}
	return "http://localhost:11434"
}

// ModelOrDefault returns the model name with a fallback.
func (c AIConfig) ModelOrDefault(fallback string) string {
	if c.Model != "" {
		return c.Model
	}
	return fallback
}
