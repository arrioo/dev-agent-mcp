package main

import (
	"encoding/json"
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

const systemPrompt = "You are a helpful Dev Agent running locally via MCP. Keep answers concise."

func main() {
	// stdout is reserved for the MCP protocol, so log to stderr from the start
	log.SetOutput(os.Stderr)

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

	log.Println("Serving MCP over stdio...")
	if err := mcpserver.ServeStdio(mcpSrv); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func startWebDashboard(monitor *agent.Monitor) {
	mux := http.NewServeMux()

	// Providers for the UI dropdown (never returns keys, only whether one is in env)
	mux.HandleFunc("/api/providers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, llm.List())
	})

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
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		if req.Message == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
			return
		}
		if req.Provider == "" {
			req.Provider = os.Getenv("DEFAULT_PROVIDER")
		}
		if req.Provider == "" {
			req.Provider = "openai"
		}

		// Log provider/model/text only. Never log the API key.
		monitor.RecordEvent("chat_message", map[string]interface{}{
			"user":     "dashboard_user",
			"provider": req.Provider,
			"model":    req.Model,
			"text":     req.Message,
		})

		messages := append(req.History, llm.Message{Role: "user", Content: req.Message})
		reply, err := llm.Chat(r.Context(), llm.Request{
			Provider: req.Provider,
			Model:    req.Model,
			APIKey:   req.APIKey,
			System:   systemPrompt,
			Messages: messages,
		})
		if err != nil {
			monitor.RecordEvent("chat_error", map[string]interface{}{
				"provider": req.Provider,
				"error":    err.Error(),
			})
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"reply": reply})
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": monitor.GetStatus()})
	})

	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, monitor.GetEvents())
	})

	mux.Handle("/", http.FileServer(http.Dir("web")))

	port := os.Getenv("PORT")
	if port == "" {
		port = "2000"
	}

	log.Printf("Starting web dashboard on http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Web Dashboard failed: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}