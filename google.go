package mockllm

import (
	"encoding/json"
	"fmt"
	"net/http"

	"google.golang.org/genai"
)

// GoogleProvider handles Google request/response mocking
type GoogleProvider struct {
	mocks []GoogleMock
}

type GoogleRequestBody struct {
	Contents []genai.Content `json:"contents"`
}

// NewGoogleProvider creates a new Google GoogleProvider with the given mocks.
func NewGoogleProvider(mocks []GoogleMock) *GoogleProvider {
	return &GoogleProvider{mocks: mocks}
}

// Handle processes a Google request.
func (p *GoogleProvider) Handle(w http.ResponseWriter, r *http.Request) {
	var requestBody GoogleRequestBody
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	mock := p.findMatchingMock(requestBody)
	if mock == nil {
		http.Error(w, "No matching mock found", http.StatusNotFound)
		return
	}

	handleNonStreamingResponse(w, mock.Response)
}

// findMatchingMock finds the first mock that matches the request.
func (p *GoogleProvider) findMatchingMock(request GoogleRequestBody) *GoogleMock {
	for _, mock := range p.mocks {
		if p.requestsMatch(mock.Match, request) {
			return &mock
		}
	}

	return nil
}

// requestsMatch checks if two requests are equivalent
func (p *GoogleProvider) requestsMatch(expected GoogleRequestMatch, actual GoogleRequestBody) bool {
	if len(actual.Contents) == 0 {
		return false
	}

	return compareMessages(expected.MatchType, expected.Content, actual.Contents[len(actual.Contents)-1])
}
