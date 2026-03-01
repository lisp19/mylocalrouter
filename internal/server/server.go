package server

import (
	"agentic-llm-gateway/pkg/logger"
	"encoding/json"
	"net/http"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/internal/providers"
	"agentic-llm-gateway/internal/router"
)

// StrategyManager is the read-only interface the Server needs from RemoteManager.
// Using an interface keeps the server package decoupled from config internals
// and simplifies unit testing.
type StrategyManager interface {
	GetStrategy() *config.RemoteStrategy
}

// Server encapsulates the HTTP handler and routing logic
type Server struct {
	rm     StrategyManager
	engine router.StrategyEngine
}

// NewServer initialises the HTTP gateway.
func NewServer(rm StrategyManager, engine router.StrategyEngine) *Server {
	return &Server{
		rm:     rm,
		engine: engine,
	}
}

// Start starts the standard library net/http server
func (s *Server) Start(addr string) error {
	// Go 1.24 enhanced routing
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	logger.Printf("[Server] Starting edge gateway on %s", addr)
	return server.ListenAndServe()
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req models.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request JSON", http.StatusBadRequest)
		return
	}

	strategy := s.rm.GetStrategy()

	provider, targetModel, err := s.engine.SelectProvider(&req, strategy)
	if err != nil {
		logger.Printf("[Server] Routing failed: %v", err)
		http.Error(w, "Internal Routing Error", http.StatusInternalServerError)
		return
	}

	// Update the request's mapped model
	req.Model = targetModel
	logger.Printf("[Server] Selected Provider: %s. Overriding model to: %s. Stream: %v", provider.Name(), targetModel, req.Stream)

	if req.Stream {
		s.handleStream(w, r, provider, &req)
	} else {
		s.handleSync(w, r, provider, &req)
	}
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request, provider providers.Provider, req *models.ChatCompletionRequest) {
	// Need context timeout? Usually upstream manages it or client aborts
	resp, err := provider.ChatCompletion(r.Context(), req)
	if err != nil {
		logger.Printf("[Server] Upstream Error (%s): %v", provider.Name(), err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request, provider providers.Provider, req *models.ChatCompletionRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	streamChan := make(chan *models.ChatCompletionStreamResponse)

	err := provider.ChatCompletionStream(r.Context(), req, streamChan)
	if err != nil {
		logger.Printf("[Server] Upstream Stream Init Error (%s): %v", provider.Name(), err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case chunk, ok := <-streamChan:
			if !ok {
				// Channel closed, output DONE
				w.Write([]byte("data: [DONE]\n\n"))
				flusher.Flush()
				return
			}

			data, _ := json.Marshal(chunk)
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
