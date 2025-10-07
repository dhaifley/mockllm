package mockllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// Server is the main mock LLM server
type Server struct {
	config            Config
	openaiProvider    *OpenAIProvider
	anthropicProvider *AnthropicProvider
	googleProvider    *GoogleProvider
	router            *mux.Router
	listener          net.Listener
}

// NewServer creates a new mock LLM server with the given config
func NewServer(config Config) *Server {
	// Convert config to provider mocks
	var openaiMocks []OpenAIMock
	for _, mock := range config.OpenAI {
		openaiMocks = append(openaiMocks, OpenAIMock{
			Name:     mock.Name,
			Match:    mock.Match,
			Response: mock.Response,
		})
	}

	var anthropicMocks []AnthropicMock
	for _, mock := range config.Anthropic {
		anthropicMocks = append(anthropicMocks, AnthropicMock{
			Name:     mock.Name,
			Match:    mock.Match,
			Response: mock.Response,
		})
	}

	var googleMocks []GoogleMock
	for _, mock := range config.Google {
		googleMocks = append(googleMocks, GoogleMock{
			Name:     mock.Name,
			Match:    mock.Match,
			Response: mock.Response,
		})
	}

	return &Server{
		config:            config,
		openaiProvider:    NewOpenAIProvider(openaiMocks),
		anthropicProvider: NewAnthropicProvider(anthropicMocks),
		googleProvider:    NewGoogleProvider(googleMocks),
	}
}

// LoadConfigFromFile loads configuration from a JSON file
func LoadConfigFromFile(path string, filesys fs.ReadFileFS) (Config, error) {
	data, err := filesys.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	return config, nil
}

// Start starts the server on a random available port and returns the base URL
func (s *Server) Start(ctx context.Context) (string, error) {
	s.setupRoutes()

	listenAddr := s.config.ListenAddr
	if listenAddr == "" {
		listenAddr = "0.0.0.0:0"
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return "", fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = listener

	go func() {
		if err := http.Serve(listener, s.router); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	if err := RetryWithBackoff(
		ctx, 5, 500*time.Millisecond, 5*time.Second, func() error {
			resp, err := http.Get(fmt.Sprintf("http://%s/health", listener.Addr().String()))
			if err != nil {
				return err
			}
			defer resp.Body.Close() //nolint:errcheck
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check failed: %d", resp.StatusCode)
			}
			return nil
		}); err != nil {
		return "", fmt.Errorf("failed to health check server: %w", err)
	}

	baseURL := fmt.Sprintf("http://%s", listener.Addr().String())
	return baseURL, nil
}

// Stop stops the server
func (s *Server) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) setupRoutes() {
	r := mux.NewRouter()

	// Health check
	r.HandleFunc("/health", s.handleHealth).Methods("GET")

	// OpenAI Chat Completions API
	r.HandleFunc("/v1/chat/completions", s.openaiProvider.Handle).Methods("POST")

	// Anthropic Messages API
	r.HandleFunc("/v1/messages", s.anthropicProvider.Handle).Methods("POST")

	// Google Generate Content API
	r.HandleFunc("/v1beta/models/{model}:generateContent", s.googleProvider.Handle).Methods("POST")

	// Debug route
	r.NotFoundHandler = http.HandlerFunc(s.handleNotFound)

	s.router = r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":    "healthy",
		"service":   "mock-llm",
		"openai":    len(s.config.OpenAI),
		"anthropic": len(s.config.Anthropic),
		"google":    len(s.config.Google),
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)

	if err := json.NewEncoder(w).Encode(map[string]any{
		"error":  "Endpoint not found",
		"path":   r.URL.Path,
		"method": r.Method,
		"hint":   "Supported: /v1/chat/completions (OpenAI), /v1/messages (Anthropic), /v1beta/models/{model}:generateContent (Google)",
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

// sliceContains checks if slice a completely contains slice b.
func sliceContains(a, b []any) bool {
	if len(b) == 0 {
		return true
	}

	if len(a) != len(b) {
		return false
	}

	for i, itemB := range b {
		switch itemB := itemB.(type) {
		case nil:
			if a[i] != nil {
				return false
			}
		case map[string]any:
			mapA, okA := a[i].(map[string]any)
			if !okA || !mapContains(mapA, itemB) {
				return false
			}
		case []any:
			sliceA, okA := a[i].([]any)
			if !okA || !sliceContains(sliceA, itemB) {
				return false
			}
		default:
			ab, err := json.Marshal(a[i])
			if err != nil {
				return false
			}

			bb, err := json.Marshal(itemB)
			if err != nil {
				return false
			}

			if !bytes.Equal(ab, bb) {
				return false
			}
		}
	}

	return true
}

// mapContains checks if map a completely contains map b.
func mapContains(a, b map[string]any) bool {
	if len(b) == 0 {
		return true
	}

	for keyB, valB := range b {
		valA, ok := a[keyB]
		if !ok {
			return false
		}

		switch v := valB.(type) {
		case nil:
			if valA != nil {
				return false
			}
		case map[string]any:
			mapA, okA := valA.(map[string]any)
			if !okA || !mapContains(mapA, v) {
				return false
			}
		case []any:
			sliceA, okA := valA.([]any)
			if !okA || !sliceContains(sliceA, v) {
				return false
			}
		default:
			ab, err := json.Marshal(valA)
			if err != nil {
				return false
			}

			bb, err := json.Marshal(valB)
			if err != nil {
				return false
			}

			if !bytes.Equal(ab, bb) {
				return false
			}
		}
	}

	return true
}

// compareMessages compares two messages based on the match type.
func compareMessages(matchType MatchType, expected any, actual any) bool {
	jsonExpected, err := json.Marshal(expected)
	if err != nil {
		return false
	}

	jsonActual, err := json.Marshal(actual)
	if err != nil {
		return false
	}

	switch matchType {
	case MatchTypeExact:
		return bytes.Equal(jsonExpected, jsonActual)
	case MatchTypeContains:
		var expectedVal, actualVal any
		if err := json.Unmarshal(jsonExpected, &expectedVal); err != nil {
			return false
		}
		if err := json.Unmarshal(jsonActual, &actualVal); err != nil {
			return false
		}

		if sv, ok := expectedVal.(string); ok {
			return bytes.Contains(jsonActual, []byte(sv))
		}

		switch expectedVal := expectedVal.(type) {
		case map[string]any:
			actualMap, ok := actualVal.(map[string]any)
			if !ok {
				return false
			}

			return mapContains(actualMap, expectedVal)
		case []any:
			actualSlice, ok := actualVal.([]any)
			if !ok {
				return false
			}

			return sliceContains(actualSlice, expectedVal)
		default:
			return bytes.Contains(jsonActual, jsonExpected)
		}
	default:
		return false
	}
}

// handleNonStreamingResponse sends a JSON response.
func handleNonStreamingResponse(w http.ResponseWriter, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}
