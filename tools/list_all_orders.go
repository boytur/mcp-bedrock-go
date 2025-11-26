// Tool: ListAllOrders
// คำอธิบาย (ไทย): คืนรายการ Manufacturing Orders ทั้งหมด (ยกเว้นสถานะ 'done') เป็น JSON
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	odoolib "mcp-bedrock-go/odoo"
)

// Input: optional filter
// Output: JSON array of manufacturing orders (excluding done)
func ListAllOrders(oclient *odoolib.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		fields := []string{"id", "name", "product_id", "product_qty", "date_deadline", "state", "workorder_ids"}
		domain := []any{[]any{"state", "!=", "done"}}

		items, err := oclient.SearchRead("mrp.production", fields, domain)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo error: %v", err)), nil
		}

		b, _ := json.MarshalIndent(items, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}
