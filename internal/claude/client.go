package claude

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/xenbyte/asterisk/internal/analysis"
)

type ImageInput struct {
	Base64    string
	MediaType string
}

type Client struct {
	api anthropic.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		api: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

// AnalyzePages sends one or more page images to Claude as a single analysis
// request. Multiple images are treated as consecutive pages of the same book.
func (c *Client) AnalyzePages(ctx context.Context, images []ImageInput, prompt PromptContext) (*analysis.Response, error) {
	systemPrompt, err := BuildSystemPrompt(prompt)
	if err != nil {
		return nil, fmt.Errorf("building system prompt: %w", err)
	}

	resp, err := c.callClaude(ctx, systemPrompt, images, "")
	if err != nil {
		return nil, err
	}

	parsed, err := analysis.Parse(resp)
	if err != nil {
		resp, retryErr := c.callClaude(ctx, systemPrompt, images,
			"Your previous response was not valid JSON. Respond with ONLY valid JSON matching the schema. No markdown fences, no commentary, no text outside the JSON object.")
		if retryErr != nil {
			return nil, fmt.Errorf("retry also failed: %w (original: %w)", retryErr, err)
		}
		parsed, err = analysis.Parse(resp)
		if err != nil {
			return nil, fmt.Errorf("JSON invalid after retry: %w", err)
		}
	}

	return parsed, nil
}

func (c *Client) callClaude(ctx context.Context, systemPrompt string, images []ImageInput, extraInstruction string) (string, error) {
	var userBlocks []anthropic.ContentBlockParamUnion
	for _, img := range images {
		userBlocks = append(userBlocks, anthropic.NewImageBlockBase64(img.MediaType, img.Base64))
	}

	instruction := "Analyze this page."
	if len(images) > 1 {
		instruction = fmt.Sprintf("These %d images are consecutive pages from the same book. Analyze them together as one combined passage — produce a single unified analysis, not separate analyses per image.", len(images))
	}
	userBlocks = append(userBlocks, anthropic.NewTextBlock(instruction))

	if extraInstruction != "" {
		userBlocks = append(userBlocks, anthropic.NewTextBlock(extraInstruction))
	}

	message, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(userBlocks...),
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude API call: %w", err)
	}

	for _, block := range message.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			return tb.Text, nil
		}
	}

	return "", fmt.Errorf("claude returned no text content")
}
