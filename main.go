package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	bedrockruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type titanResponse struct {
	Results []struct {
		OutputText string `json:"outputText"`
	} `json:"results"`
}

func logErr(format string, v ...any) {
	// write to stderr only
	fmt.Fprintf(os.Stderr, format+"\n", v...)
}

func main() {
	// Load AWS config
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		logErr("AWS load error: %v", err)
		os.Exit(1)
	}

	br := bedrockruntime.NewFromConfig(cfg)
	modelID := "us.meta.llama3-1-70b-instruct-v1:0'"

	// Create MCP server
	s := server.NewMCPServer(
		"bedrock-mcp",
		"0.1.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	// Define tool
	tool := mcp.NewTool(
		"bedrock_chat",
		mcp.WithDescription("Send a prompt to Amazon Bedrock"),
		mcp.WithString("prompt", mcp.Required()),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		prompt, _ := req.RequireString("prompt")

		body, _ := json.Marshal(map[string]any{
			"inputText": prompt,
			"textGenerationConfig": map[string]any{
				"maxTokenCount": 300,
				"temperature":   0.7,
			},
		})

		resp, err := br.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(modelID),
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
			Body:        body,
		})

		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var titan titanResponse
		if err := json.Unmarshal(resp.Body, &titan); err != nil {
			return mcp.NewToolResultError("decode error"), nil
		}

		if len(titan.Results) == 0 {
			return mcp.NewToolResultError("empty result"), nil
		}

		return mcp.NewToolResultText(titan.Results[0].OutputText), nil
	})

	// Start server (stdout must be clean)
	if err := server.ServeStdio(s); err != nil {
		logErr("Serve error: %v", err)
	}
}
