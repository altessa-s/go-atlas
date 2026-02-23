// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	claudeAPIURL     = "https://api.anthropic.com/v1/messages"
	claudeModel      = "claude-3-5-sonnet-20241022"
	claudeAPIVersion = "2023-06-01"
	maxTokens        = 4096
	requestTimeout   = 60 * time.Second
)

// ClaudeClient sends batched prompts to the Anthropic Messages API to generate
// short descriptions for environment variables. It is not safe for concurrent use.
type ClaudeClient struct {
	httpClient *http.Client
	apiKey     string
}

// NewClaudeClient returns a client configured with apiKey and a default
// HTTP transport (60 s request timeout, connection pooling).
func NewClaudeClient(apiKey string) *ClaudeClient {
	return &ClaudeClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,               //nolint:mnd
				MaxIdleConnsPerHost: 2,                //nolint:mnd
				IdleConnTimeout:     90 * time.Second, //nolint:mnd
			},
		},
	}
}

// claudeRequest represents a request to Claude API.
type claudeRequest struct {
	Messages  []claudeMessage `json:"messages"`
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
}

// claudeMessage represents a message in the conversation.
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse represents a response from Claude API.
type claudeResponse struct {
	Error   *claudeError    `json:"error,omitempty"`
	Content []claudeContent `json:"content"`
}

// claudeContent represents content in the response.
type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// claudeError represents an error from Claude API.
type claudeError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// GenerateDescriptions sends a single prompt containing all envVars for the given
// sectionName and returns a map of variable name to short description (max 10 words).
// The response is expected as a JSON object; markdown code fences are stripped
// before unmarshalling. Returns an error if the API call fails or the response
// cannot be parsed as JSON.
func (c *ClaudeClient) GenerateDescriptions(envVars map[string]varInfo, sectionName string) (map[string]string, error) {
	// Build prompt with all variables in the section
	var promptBuilder strings.Builder
	promptBuilder.WriteString(fmt.Sprintf(
		"Generate concise descriptions (max 10 words) for the following %s configuration environment variables.\n\n",
		formatSectionName(sectionName)))
	promptBuilder.WriteString("For each variable, provide a brief description that explains its purpose.\n")
	promptBuilder.WriteString("Format your response as a JSON object with variable names as keys and descriptions as values.\n\n")
	promptBuilder.WriteString("Environment variables:\n\n")

	keys := make([]string, 0, len(envVars))
	for key := range envVars {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		info := envVars[key]
		promptBuilder.WriteString(fmt.Sprintf("- %s (type: %s, default: %s)\n", key, info.Type, info.Value))
	}

	promptBuilder.WriteString("\nProvide ONLY the JSON object, without any additional text or markdown formatting.")

	// Create request
	req := claudeRequest{
		Messages: []claudeMessage{
			{
				Role:    "user",
				Content: promptBuilder.String(),
			},
		},
		Model:     claudeModel,
		MaxTokens: maxTokens,
	}

	// Send request
	responseText, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	// Clean response text - remove markdown code blocks if present
	cleanedText := strings.TrimSpace(responseText)
	if strings.HasPrefix(cleanedText, "```json") {
		cleanedText = strings.TrimPrefix(cleanedText, "```json")
		cleanedText = strings.TrimPrefix(cleanedText, "```")
		cleanedText = strings.TrimSuffix(cleanedText, "```")
		cleanedText = strings.TrimSpace(cleanedText)
	} else if strings.HasPrefix(cleanedText, "```") {
		cleanedText = strings.TrimPrefix(cleanedText, "```")
		cleanedText = strings.TrimSuffix(cleanedText, "```")
		cleanedText = strings.TrimSpace(cleanedText)
	}

	// Parse JSON response
	descriptions := make(map[string]string)
	if err := json.Unmarshal([]byte(cleanedText), &descriptions); err != nil {
		return nil, coreerrs.Wrapf(err, "failed to parse Claude response (response: %s)", cleanedText)
	}

	return descriptions, nil
}

// sendRequest sends a request to Claude API and returns the response text.
func (c *ClaudeClient) sendRequest(req claudeRequest) (string, error) {
	// Marshal request body
	body, err := json.Marshal(req)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "marshal request")
	}

	// Create HTTP request with context
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", coreerrs.WrapOperation(err, "create request")
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", claudeAPIVersion)

	// Send request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "send request")
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "read response")
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return "", coreerrs.WrapOperation(err, "unmarshal response")
	}

	// Check for API errors
	if claudeResp.Error != nil {
		return "", fmt.Errorf("API error: %s - %s", claudeResp.Error.Type, claudeResp.Error.Message)
	}

	// Extract text from content
	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return claudeResp.Content[0].Text, nil
}
