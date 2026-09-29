package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"dev-agent-mcp/internal/agent"
	"dev-agent-mcp/internal/llm"
	"dev-agent-mcp/internal/mcp"
	"dev-agent-mcp/internal/storage"

	"github.com/joho/godotenv"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func main() {
	// Load .env variables (like MCP_API_KEY)
	_ = godotenv.Load()

	store, err := storage.NewFileStore("storage/temp/data.json")
	if err != nil {
		log.Fatalf("Failed to initialize file store: %v", err)
	}

	monitor := agent.NewMonitor(store)
	if monitor.GetStatus() == "" || monitor.GetStatus() == "offline" {
		monitor.SetStatus("online")
	}
	monitor.RecordEvent("startup", map[string]interface{}{
		"message": "Dashboard & MCP Server started",
	})

	mcpSrv := mcp.NewMCPServer(monitor)

	go startWebDashboard(monitor)

	log.SetOutput(os.Stderr)
	log.Println("Serving MCP over stdio...")
	if err := mcpserver.ServeStdio(mcpSrv); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func startWebDashboard(monitor *agent.Monitor) {
	mux := http.NewServeMux()

	// Chat API Endpoint
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Provider string        `json:"provider"`
			Model    string        `json:"model"`
			APIKey   string        `json:"api_key"`
			Message  string        `json:"message"`
			History  []llm.Message `json:"history"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		if req.Provider == "" {
			req.Provider = os.Getenv("DEFAULT_PROVIDER")
		}

		// Do NOT log api_key
		monitor.RecordEvent("chat_message", map[string]interface{}{
			"user":     "dashboard_user",
			"provider": req.Provider,
			"model":    req.Model,
			"text":     req.Message,
		})

		reply, err := llm.Chat(r.Context(), llm.Request{
			Provider: req.Provider,
			Model:    req.Model,
			APIKey:   req.APIKey,
			System:   "You are a helpful Dev Agent running locally via MCP. Keep answers concise.",
			Messages: append(req.History, llm.Message{Role: "user", Content: req.Message}),
		})

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"reply": reply})
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": monitor.GetStatus(),
		})
	})

	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(monitor.GetEvents())
	})

	// Providers for the UI dropdown (never returns keys, only whether one is in env)
	mux.HandleFunc("/api/providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.List())
	})

	fs := http.FileServer(http.Dir("web"))
	mux.Handle("/", fs)

	port := os.Getenv("PORT")
	if port == "" {
		port = "2000"
	}

	log.Printf("Starting web dashboard on http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Web Dashboard failed: %v", err)
	}
}

func processLLMChat(userMessage string) string {
	apiKey := os.Getenv("MCP_API_KEY")
	if apiKey == "" {
		if userMessage == "ping" {
			return "pong! (No LLM API key configured in .env)"
		}
		return "Please set MCP_API_KEY in .env to enable AI chat."
	}

	// This makes a standard call to OpenAI.
	// To support Claude/Gemini dynamically, you could check the prefix of the key 
	// or use a service like OpenRouter.
	url := "https://api.openai.com/v1/chat/completions"
	
	reqBody := map[string]interface{}{
		"model": "gpt-4o-mini", // Or gpt-4o
		"messages": []map[string]string{
			{"role": "system", "content": "You are a helpful Dev Agent running locally via MCP. Keep answers concise."},
			{"role": "user", "content": userMessage},
		},
	}
	
	jsonData, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "Error creating request: " + err.Error()
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "Network error calling LLM: " + err.Error()
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "Error parsing LLM response"
	}

	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := msg["content"].(string); ok {
					return content
				}
			}
		}
	}
	
	// Check for API errors
	if apiErr, ok := result["error"].(map[string]interface{}); ok {
		if msg, ok := apiErr["message"].(string); ok {
			return "API Error: " + msg
		}
	}

	return "LLM returned an unexpected response format."
}
