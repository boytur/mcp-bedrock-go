package bedrock

import (
	"context"
	"encoding/json"
	"fmt"

	bedrockruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// Client wraps the AWS Bedrock runtime client with a small helper.
type Client struct {
	inner *bedrockruntime.Client
}

// New creates a wrapper around the provided bedrockruntime client.
func New(inner *bedrockruntime.Client) *Client {
	return &Client{inner: inner}
}

// GenerateText invokes the model with a raw prompt and returns textual output.
func (c *Client) GenerateText(ctx context.Context, modelID string, prompt any) (string, error) {
	var bodyBytes []byte
	switch v := prompt.(type) {
	case string:
		// most Bedrock models expect an object with a string prompt field
		payload := map[string]any{"prompt": v}
		b, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		bodyBytes = b
	default:
		b, err := json.Marshal(prompt)
		if err != nil {
			return "", err
		}
		bodyBytes = b
	}

	resp, err := c.inner.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     &modelID,
		ContentType: awsString("application/json"),
		Accept:      awsString("application/json"),
		Body:        bodyBytes,
	})
	if err != nil {
		return "", err
	}

	// Best-effort: return raw body if we can't parse a structured response
	var out any
	_ = json.Unmarshal(resp.Body, &out)
	switch v := out.(type) {
	case map[string]any:
		// try to extract common fields
		if gen, ok := v["generation"]; ok {
			if s, ok := gen.(string); ok {
				return s, nil
			}
		}
	}
	return string(resp.Body), nil
}

func awsString(s string) *string { return &s }

// FormatSystemPrompt wraps the user's message into the system prompt template
// requested by the user. It returns a single string which should be passed
// to GenerateText.
func FormatSystemPrompt(userMessage string) string {
	return fmt.Sprintf("<|begin_of_text|><|start_header_id|>user<|end_header_id|>\n%s\n<|eot_id|>\n<|start_header_id|>assistant<|end_header_id|>\n", userMessage)
}
