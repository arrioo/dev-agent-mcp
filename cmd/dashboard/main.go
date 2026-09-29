package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"dev-agent-mcp/internal/agent"
	"dev-agent-mcp/internal/mcp"
	"dev-agent-mcp/internal/storage"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

func main() {
	// 1. Initialize File Storage in a local temp folder (not Windows temp)
	// It will save to ./storage/temp/data.json inside your project.
	store, err := storage.NewFileStore("storage/temp/data.json")
	if err != nil {
		log.Fatalf("Failed to initialize file store: %v", err)
	}

	// 2. Initialize Agent Monitor
	monitor := agent.NewMonitor(store)
	if monitor.GetStatus() == "" || monitor.GetStatus() == "offline" {
		monitor.SetStatus("online")
	}
	monitor.RecordEvent("startup", map[string]interface{}{
		"message": "Dashboard & MCP Server started",
	})

	// 3. Create the MCP Server instance
	mcpSrv := mcp.NewMCPServer(monitor)

	// 4. Start the Web Dashboard in a background goroutine
	go startWebDashboard(monitor)

	// 5. Start the MCP Server over Stdio in the main thread
	// This ensures the binary acts as a standard MCP server for Claude/Cursor!
	// Important: We send logs to Stderr to prevent corrupting the Stdio JSON-RPC.
	log.SetOutput(os.Stderr)
	log.Println("Serving MCP over stdio...")
	if err := mcpserver.ServeStdio(mcpSrv); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func startWebDashboard(monitor *agent.Monitor) {
	mux := http.NewServeMux()

	// API Handlers for the Frontend
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		// Record the user's message as an event
		monitor.RecordEvent("chat_message", map[string]interface{}{
			"user": "dashboard_user",
			"text": req.Message,
		})

		// Simple command processing (simulating agent intelligence)
		reply := "I received your message: " + req.Message
		if req.Message == "ping" {
			reply = "pong! My current status is: " + monitor.GetStatus()
		} else if req.Message == "status" {
			reply = "Agent is " + monitor.GetStatus()
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"reply": reply,
		})
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

	// Serve the Website (Static HTML/CSS/JS)
	fs := http.FileServer(http.Dir("web"))
	mux.Handle("/", fs)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting web dashboard on http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Web Dashboard failed: %v", err)
	}
}
