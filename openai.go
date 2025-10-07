package mockllm

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/openai/openai-go"
)

// Provider handles OpenAI request/response mocking
type OpenAIProvider struct {
	mocks []OpenAIMock
}

// NewOpenAIProvider creates a new OpenAI OpenAIProvider with the given mocks
func NewOpenAIProvider(mocks []OpenAIMock) *OpenAIProvider {
	return &OpenAIProvider{mocks: mocks}
}

// Handle processes an OpenAI chat completion request
func (p *OpenAIProvider) Handle(w http.ResponseWriter, r *http.Request) {
	// Parse the incoming request into SDK type
	var requestBody openai.ChatCompletionNewParams
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Find a matching mock
	mock := p.findMatchingMock(requestBody)
	if mock == nil {
		http.Error(w, "No matching mock found.", http.StatusNotFound)
		return
	}

	// Return the response
	handleNonStreamingResponse(w, mock.Response)
}

// findMatchingMock finds the first mock that matches the request
func (p *OpenAIProvider) findMatchingMock(request openai.ChatCompletionNewParams) *OpenAIMock {
	for _, mock := range p.mocks {
		if p.requestsMatch(mock.Match, request) {
			return &mock
		}
	}
	return nil
}

// requestsMatch checks if two requests are equivalent
func (p *OpenAIProvider) requestsMatch(expected OpenAIRequestMatch, actual openai.ChatCompletionNewParams) bool {
	if len(actual.Messages) == 0 {
		return false
	}

	return compareMessages(expected.MatchType, expected.Message, actual.Messages[len(actual.Messages)-1])
}
