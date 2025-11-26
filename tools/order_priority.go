// Tool: OrderPriority
// คำอธิบาย (ไทย): ให้คะแนนหรือเรียงลำดับ Manufacturing Orders (MO) ตามเงื่อนไข เช่น ปริมาณ หรือลำดับความสำคัญ
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	odoolib "mcp-bedrock-go/odoo"
)

// Input: none or optional list of mo_ids
// Output: JSON ranked list of orders with score and reason
func OrderPriority(oclient *odoolib.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// For demo: rank by product_qty descending
		items, err := oclient.SearchRead("mrp.production", []string{"id", "name", "product_qty", "date_deadline", "state"}, []any{[]any{}})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo error: %v", err)), nil
		}
		// return raw items — consumer can compute ranking client-side or we could score
		b, _ := json.MarshalIndent(items, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}
