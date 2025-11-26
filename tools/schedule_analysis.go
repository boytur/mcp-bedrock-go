// Tool: ScheduleAnalysis
// คำอธิบาย (ไทย): วิเคราะห์ผลกระทบด้านตารางการผลิตตามโปรไฟล์ที่ระบุ (เช่น Cost-Aware, Throughput) โดยใช้บริบทจาก Odoo และ LLM
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	bedrocklib "mcp-bedrock-go/bedrock"
	odoolib "mcp-bedrock-go/odoo"
)

// Input: profile (string) — e.g., "Cost-Aware" or "Throughput"
// Output: text analysis from LLM considering current Odoo context
func ScheduleAnalysis(oclient *odoolib.Client, br *bedrocklib.Client, modelID string) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		profile, err := req.RequireString("profile")
		if err != nil {
			profile = "Balanced"
		}

		// Gather small context
		moFields := []string{"id", "name", "product_id", "product_qty", "date_deadline", "state"}
		mos, _ := oclient.SearchRead("mrp.production", moFields, []any{[]any{"state", "in", []string{"confirmed", "progress"}}})

		prodFields := []string{"id", "default_code", "name"}
		prods, _ := oclient.SearchRead("product.product", prodFields, []any{[]any{}})

		ctxObj := map[string]any{"manufacturing_orders": mos, "products": prods}

		prompt := fmt.Sprintf("Profile: %s\nContext: %s\n\nTask: A new RUSH order (Product Code: RUSH-TEA, Qty:1000) has arrived. Provide a concise recommendation and rationale.", profile, mustJSON(ctxObj))

		// Wrap prompt with system template and send as string
		promptText := bedrocklib.FormatSystemPrompt(prompt)
		out, err := br.GenerateText(ctx, modelID, promptText)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("LLM error: %v", err)), nil
		}
		return mcp.NewToolResultText(out), nil
	}
}

func mustJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
