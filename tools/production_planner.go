package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	bedrocklib "mcp-bedrock-go/bedrock"
	odoolib "mcp-bedrock-go/odoo"
)

// Input: mo_id (int)
// Output: textual production plan suggestion (LLM-generated) and JSON plan
func ProductionPlanner(oclient *odoolib.Client, br *bedrocklib.Client, modelID string) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		moid := req.GetInt("mo_id", 0)
		if moid == 0 {
			return mcp.NewToolResultError("mo_id is required"), nil
		}

		mos, err := oclient.SearchRead("mrp.production", []string{"id", "name", "product_id", "product_qty", "date_deadline"}, []any{[]any{"id", "=", moid}})
		if err != nil || len(mos) == 0 {
			return mcp.NewToolResultError(fmt.Sprintf("MO not found or Odoo error: %v", err)), nil
		}

		// Prepare prompt as plain string (Bedrock requires "prompt" to be a string)
		moJson, _ := json.MarshalIndent(mos[0], "", "  ")
		promptText := fmt.Sprintf("Generate short production plan with workcenter assignment and estimated duration.\n\nMO:\n%s", string(moJson))
		// Wrap with system prompt template required by model
		promptText = bedrocklib.FormatSystemPrompt(promptText)
		out, err := br.GenerateText(ctx, modelID, promptText)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("LLM error: %v", err)), nil
		}

		// Return both LLM text and structured MO for reference
		resp := map[string]any{"plan_text": out, "mo": mos[0]}
		b, _ := json.MarshalIndent(resp, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}
