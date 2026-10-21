package apichat

// OpenAI-compatible chat transport. Works with OpenAI, Featherless, Groq,
// OpenRouter, DeepSeek, Gemini's OpenAI endpoint and anything else that speaks
// /chat/completions.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// chat performs one /chat/completions call and returns the message text.
func (e *Engine) chat(ctx context.Context, msgs []Message) (string, error) {
	payload := map[string]any{
		"model":       e.cfg.Model,
		"messages":    msgs,
		"temperature": e.cfg.Temperature,
		"max_tokens":  e.cfg.MaxTokens,
		"stop":        []string{"<|end|>", "<|eot_id|>", "<|im_end|>", "<|endoftext|>", "</s>"},
	}
	if !e.cfg.DisableNoThink {
		for key, value := range noThinkFields(e.cfg.Model) {
			payload[key] = value
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	if e.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.cfg.Timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)

	client := e.cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: e.cfg.Timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var parsed chatResponse
	jsonErr := json.Unmarshal(data, &parsed)
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(data))
		if jsonErr == nil && parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", fmt.Errorf("API %d: %s", resp.StatusCode, msg)
	}
	if jsonErr != nil {
		return "", fmt.Errorf("bad API response: %v", jsonErr)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("API returned no choices")
	}
	choice := parsed.Choices[0]
	content := strings.TrimSpace(choice.Message.Content)
	if strings.TrimSpace(thinkRe.ReplaceAllString(content, "")) == "" {
		if strings.TrimSpace(choice.Message.ReasoningContent) != "" || strings.Contains(content, "<think>") {
			return "", errors.New("model spent all tokens thinking and wrote no reply (raise max tokens or use a non-thinking model)")
		}
	}
	if content == "" {
		return "", errors.New("API returned an empty reply")
	}
	return content, nil
}

// noThinkFields switches off "thinking" for known hybrid-reasoning families so
// the model spends its tokens on the chat line, not on reasoning.
func noThinkFields(model string) map[string]any {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "qwen3"), strings.Contains(m, "qwen-3"), strings.Contains(m, "qwen4"):
		return map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": false}}
	case strings.Contains(m, "glm-4.5"), strings.Contains(m, "glm-4.6"), strings.Contains(m, "glm-4.7"), strings.Contains(m, "glm-5"), strings.Contains(m, "glm5"):
		return map[string]any{"thinking": map[string]any{"type": "disabled"}, "chat_template_kwargs": map[string]any{"enable_thinking": false}}
	case strings.Contains(m, "deepseek-v3.1"), strings.Contains(m, "deepseek-v3.2"), strings.Contains(m, "deepseek-v4"), strings.Contains(m, "deepseek-r"):
		return map[string]any{"chat_template_kwargs": map[string]any{"thinking": false, "enable_thinking": false}, "thinking": map[string]any{"type": "disabled"}}
	case strings.Contains(m, "kimi-k2.5"), strings.Contains(m, "kimi-k3"), strings.Contains(m, "kimi-k2-thinking"):
		return map[string]any{"thinking": map[string]any{"type": "disabled"}, "chat_template_kwargs": map[string]any{"thinking": false, "enable_thinking": false}}
	case strings.Contains(m, "gpt-oss"):
		return map[string]any{"reasoning_effort": "low"}
	case strings.Contains(m, "minimax-m"), strings.Contains(m, "mimo"), strings.Contains(m, "step-3"), strings.Contains(m, "nemotron"):
		return map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": false, "thinking": false}}
	}
	return nil
}

// clampTimeout keeps a zero/negative timeout from disabling the client.
func clampTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return 25 * time.Second
	}
	return d
}
