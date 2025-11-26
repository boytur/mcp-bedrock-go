// Tool: ListActiveProducts
// คำอธิบาย (ไทย): ดึงรายการการผลิต (MO) ที่กำลังทำงานหรือยืนยันไว้ คืนค่าเป็นอาเรย์ JSON
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	odoolib "mcp-bedrock-go/odoo"
)

// Input: none
// Output: JSON array of active manufacturing orders
func ListActiveProducts(oclient *odoolib.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		fields := []string{"id", "name", "product_id", "product_qty", "date_deadline", "state"}
		domain := []any{[]any{"state", "in", []string{"confirmed", "progress", "done"}}}

		items, err := oclient.SearchRead("mrp.production", fields, domain)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo error: %v", err)), nil
		}

		if len(items) == 0 {
			return mcp.NewToolResultText("[]"), nil
		}

		b, _ := json.MarshalIndent(items, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}
