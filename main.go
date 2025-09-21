package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt" // This was the missing import
	"io"
	"log"
	"net/http"
	"time"
)

// Represents the structure of a single message in the chat
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Represents the payload sent to the /api/chat endpoint
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Represents the structure of the model list from Ollama's /api/tags
type OllamaTagModel struct {
	Name       string    `json:"name"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
}

type OllamaTagsResponse struct {
	Models []OllamaTagModel `json:"models"`
}

// Serves the main index.html file.
func serveUI(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/index.html")
}

// Fetches the list of available models from the Ollama API.
func getOllamaModelsHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		http.Error(w, "Failed to connect to Ollama", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read Ollama response", http.StatusInternalServerError)
		return
	}

	var tagsResponse OllamaTagsResponse
	if err := json.Unmarshal(body, &tagsResponse); err != nil {
		http.Error(w, "Failed to parse Ollama models", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tagsResponse.Models)
}

// Handles the chat logic, streaming the response from Ollama.
func chatHandler(w http.ResponseWriter, r *http.Request) {
	var chatReq ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Prepare the request for Ollama API
	ollamaURL := "http://localhost:11434/api/chat"
	payload := map[string]interface{}{
		"model":    chatReq.Model,
		"messages": chatReq.Messages,
		"stream":   true,
	}
	
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "Error creating Ollama payload", http.StatusInternalServerError)
		return
	}

	// Make the POST request to Ollama
	resp, err := http.Post(ollamaURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		http.Error(w, "Error connecting to Ollama", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Set headers for Server-Sent Events (SSE)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Get the request context to detect client disconnects.
	ctx := r.Context()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		// This case triggers if the client has disconnected.
		case <-ctx.Done():
			log.Println("Client disconnected, stopping stream.")
			return // Exit the handler
		default:
			// This is the original streaming logic.
			line := scanner.Bytes()
			if len(line) > 0 {
				fmt.Fprintf(w, "data: %s\n\n", line)
				flusher.Flush()
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading stream from Ollama: %v", err)
	}
}


func main() {
    // Serve static files (HTML, CSS, JS) from the 'static' directory
    fs := http.FileServer(http.Dir("./static"))
    http.Handle("/", fs)

	http.HandleFunc("/api/models", getOllamaModelsHandler)
	http.HandleFunc("/api/chat", chatHandler)

	port := "8081"
	log.Printf("Starting server on http://localhost:%s", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}