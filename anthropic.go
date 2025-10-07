package mockllm

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
)

// AnthropicProvider handles Anthropic request/response mocking
type AnthropicProvider struct {
	mocks []AnthropicMock
}

// NewAnthropicProvider creates a new Anthropic AnthropicProvider with the given mocks
func NewAnthropicProvider(mocks []AnthropicMock) *AnthropicProvider {
	return &AnthropicProvider{mocks: mocks}
}

// Handle processes an Anthropic messages request
func (p *AnthropicProvider) Handle(w http.ResponseWriter, r *http.Request) {
	// Check for required headers
	if r.Header.Get("x-api-key") == "" {
		http.Error(w, "Missing x-api-key header", http.StatusUnauthorized)
		return
	}

	if r.Header.Get("anthropic-version") == "" {
		http.Error(w, "Missing anthropic-version header", http.StatusBadRequest)
		return
	}

	// Parse the incoming request into SDK type
	var requestBody anthropic.MessageNewParams
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Find a matching mock
	mock := p.findMatchingMock(requestBody)
	if mock == nil {
		http.Error(w, "No matching mock found", http.StatusNotFound)
		return
	}

	handleNonStreamingResponse(w, mock.Response)
}

// findMatchingMock finds the first mock that matches the request
func (p *AnthropicProvider) findMatchingMock(request anthropic.MessageNewParams) *AnthropicMock {
	for _, mock := range p.mocks {
		if p.requestsMatch(mock.Match, request) {
			return &mock
		}
	}
	return nil
}

// requestsMatch checks if two requests are equivalent
func (p *AnthropicProvider) requestsMatch(expected AnthropicRequestMatch, actual anthropic.MessageNewParams) bool {
	if len(actual.Messages) == 0 {
		return false
	}

	return compareMessages(expected.MatchType, expected.Message, actual.Messages[len(actual.Messages)-1])
}
