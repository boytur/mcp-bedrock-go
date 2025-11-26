// Tool: OrderRisk
// คำอธิบาย (ไทย): ประเมินความเสี่ยงของ Manufacturing Order (MO) โดยตรวจสอบสถานะและแนะนำการตรวจสอบวัสดุ
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	odoolib "mcp-bedrock-go/odoo"
)

// Input: mo_id (int)
// Output: JSON risk assessment for the given manufacturing order
func OrderRisk(oclient *odoolib.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		moid := req.GetInt("mo_id", 0)
		if moid == 0 {
			return mcp.NewToolResultError("mo_id is required"), nil
		}

		mos, err := oclient.SearchRead("mrp.production", []string{"id", "name", "product_id", "product_qty", "state"}, []any{[]any{"id", "=", moid}})
		if err != nil || len(mos) == 0 {
			return mcp.NewToolResultError(fmt.Sprintf("MO not found or Odoo error: %v", err)), nil
		}

		// Simplified static risk assessment
		risk := map[string]any{"mo_id": moid, "risk_level": "medium", "notes": "Material checks recommended"}
		b, _ := json.MarshalIndent(risk, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}
