// Package llm gives one Chat() function that works with multiple providers.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

type Request struct {
	Provider string
	Model    string // optional, falls back to the provider default
	APIKey   string // optional, falls back to the provider's env var
	System   string
	Messages []Message
}

type ProviderInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DefaultModel string `json:"default_model"`
	KeyInEnv     bool   `json:"key_in_env"`
	NeedsKey     bool   `json:"needs_key"`
}

type spec struct {
	Name         string
	Kind         string // "openai" (OpenAI-compatible), "anthropic", "gemini"
	BaseURL      string
	DefaultModel string
	EnvKey       string
	NeedsKey     bool
}

// Add any OpenAI-compatible service here with Kind "openai" and it just works.
var order = []string{"openai", "anthropic", "gemini", "openrouter", "groq", "deepseek", "ollama"}

var specs = map[string]spec{
	"openai":     {"OpenAI", "openai", "https://api.openai.com/v1", "gpt-4o-mini", "OPENAI_API_KEY", true},
	"anthropic":  {"Anthropic (Claude)", "anthropic", "https://api.anthropic.com/v1", "claude-sonnet-5-5", "ANTHROPIC_API_KEY", true},
	"gemini":     {"Google Gemini", "gemini", "https://generativelanguage.googleapis.com/v1beta", "gemini-2.5-flash", "GEMINI_API_KEY", true},
	"openrouter": {"OpenRouter", "openai", "https://openrouter.ai/api/v1", "openai/gpt-4o-mini", "OPENROUTER_API_KEY", true},
	"groq":       {"Groq", "openai", "https://api.groq.com/openai/v1", "llama-3.3-70b-versatile", "GROQ_API_KEY", true},
	"deepseek":   {"DeepSeek", "openai", "https://api.deepseek.com/v1", "deepseek-chat", "DEEPSEEK_API_KEY", true},
	"ollama":     {"Ollama (local)", "openai", "http://localhost:11434/v1", "llama3.2", "", false},
}

// List returns providers for the UI dropdown.
func List() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(order))
	for _, id := range order {
		s := specs[id]
		out = append(out, ProviderInfo{
			ID:           id,
			Name:         s.Name,
			DefaultModel: s.DefaultModel,
			KeyInEnv:     s.EnvKey != "" && os.Getenv(s.EnvKey) != "",
			NeedsKey:     s.NeedsKey,
		})
	}
	return out
}

// Chat sends the conversation to the chosen provider and returns the reply text.
func Chat(ctx context.Context, r Request) (string, error) {
	s, ok := specs[r.Provider]
	if !ok {
		return "", fmt.Errorf("unknown provider %q", r.Provider)
	}

	if r.Model == "" {
		r.Model = s.DefaultModel
	}
	
	if r.APIKey == "" && s.EnvKey != "" {
		r.APIKey = os.Getenv(s.EnvKey)
	}
	if r.APIKey == "" {
		r.APIKey = os.Getenv("MCP_API_KEY") // legacy fallback
	}
	if r.APIKey == "" && s.NeedsKey {
		return "", fmt.Errorf("no API key for %s: enter one in the UI or set %s", s.Name, s.EnvKey)
	}

	switch s.Kind {
	case "anthropic":
		return chatAnthropic(ctx, s, r)
	case "gemini":
		return chatGemini(ctx, s, r)
	default:
		return chatOpenAI(ctx, s, r)
	}
}

// ---------- OpenAI-compatible ----------

func chatOpenAI(ctx context.Context, s spec, r Request) (string, error) {
	msgs := []map[string]string{}
	if r.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": r.System})
	}
	for _, m := range r.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	headers := map[string]string{}
	if r.APIKey != "" {
		headers["Authorization"] = "Bearer " + r.APIKey
	}
	err := postJSON(ctx, s.BaseURL+"/chat/completions", headers,
		map[string]any{"model": r.Model, "messages": msgs}, &out)
	if err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty response from %s", s.Name)
	}
	return out.Choices[0].Message.Content, nil
}

// ---------- Anthropic ----------

func chatAnthropic(ctx context.Context, s spec, r Request) (string, error) {
	msgs := make([]map[string]string, 0, len(r.Messages))
	for _, m := range r.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body := map[string]any{"model": r.Model, "max_tokens": 2048, "messages": msgs}
	if r.System != "" {
		body["system"] = r.System
	}

	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	err := postJSON(ctx, s.BaseURL+"/messages", map[string]string{
		"x-api-key":         r.APIKey,
		"anthropic-version": "2023-06-01",
	}, body, &out)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("empty response from %s", s.Name)
	}
	return sb.String(), nil
}

// ---------- Gemini ----------

func chatGemini(ctx context.Context, s spec, r Request) (string, error) {
	contents := make([]map[string]any, 0, len(r.Messages))
	for _, m := range r.Messages {
		role := "user"
		if m.Role == "assistant" {
			role = "model" // Gemini's name for the assistant role
		}
		contents = append(contents, map[string]any{
			"role":  role,
			"parts": []map[string]string{{"text": m.Content}},
		})
	}
	body := map[string]any{"contents": contents}
	if r.System != "" {
		body["system_instruction"] = map[string]any{
			"parts": []map[string]string{{"text": r.System}},
		}
	}

	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	url := fmt.Sprintf("%s/models/%s:generateContent", s.BaseURL, r.Model)
	err := postJSON(ctx, url, map[string]string{"x-goog-api-key": r.APIKey}, body, &out)
	if err != nil {
		return "", err
	}
	if len(out.Candidates) == 0 {
		return "", fmt.Errorf("empty response from %s (possibly blocked by safety filters)", s.Name)
	}
	var sb strings.Builder
	for _, p := range out.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	return sb.String(), nil
}

// ---------- HTTP helper ----------

var httpClient = &http.Client{Timeout: 90 * time.Second}

func postJSON(ctx context.Context, url string, headers map[string]string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("network error: %w", err)
	}
	defer res.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("provider returned %d: %s", res.StatusCode, apiError(data))
	}
	return json.Unmarshal(data, out)
}

// apiError pulls {"error":{"message":"..."}} out of a provider response.
func apiError(data []byte) string {
	var e struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil {
		switch v := e.Error.(type) {
		case string:
			return v
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				return m
			}
		}
	}
	s := string(data)
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}