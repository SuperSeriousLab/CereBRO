// Package pipeline — eidos-llm adapter for the ML enricher.
//
// EidosLLMCaller wraps the shared eidos-llm client (SLR → Grok → Ollama fallback chain)
// and implements LLMCaller so it can be dropped into MLEnricherConfig.LLMCaller.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"time"

	llmclient "github.com/SuperSeriousLab/eidos-llm/client"
)

// DefaultEidosLLMConfig returns the standard eidos-llm client configuration.
// Fallback chain: SLR (192.168.14.69:8081) → Grok (XAI_API_KEY) → Ollama (10.70.70.14).
func DefaultEidosLLMConfig() llmclient.Config {
	ollamaEndpoint := os.Getenv("EIDOS_OLLAMA_HOST")
	if ollamaEndpoint == "" {
		ollamaEndpoint = "http://10.70.70.14:11434"
	}
	return llmclient.Config{
		SLREndpoint:    "http://192.168.14.69:8081",
		GrokEndpoint:   "https://api.x.ai/v1/chat/completions",
		GrokAPIKeyEnv:  "XAI_API_KEY",
		OllamaEndpoint: ollamaEndpoint,
		OllamaModel:    "glm-4.7-flash:q4_K_M",
		Timeout:        30 * time.Second,
	}
}

// EidosLLMCaller implements LLMCaller using the shared eidos-llm client.
// It converts a single prompt string into a user message and calls Complete
// through the SLR → Grok → Ollama fallback chain.
type EidosLLMCaller struct {
	client *llmclient.Client
	model  string
}

// NewEidosLLMCaller creates a new EidosLLMCaller with the given config.
// model is forwarded to SLR for routing (e.g. "auto", "local", or a specific model name).
func NewEidosLLMCaller(cfg llmclient.Config, model string) *EidosLLMCaller {
	if model == "" {
		model = "auto"
	}
	return &EidosLLMCaller{
		client: llmclient.New(cfg),
		model:  model,
	}
}

// Call implements LLMCaller. It sends the prompt as a single user message and
// returns the completion text.
func (e *EidosLLMCaller) Call(ctx context.Context, prompt string) (string, error) {
	msgs := []llmclient.Message{
		{Role: "user", Content: prompt},
	}
	resp, err := e.client.Complete(ctx, e.model, msgs)
	if err != nil {
		return "", fmt.Errorf("eidos-llm: %w", err)
	}
	return resp.Content, nil
}
