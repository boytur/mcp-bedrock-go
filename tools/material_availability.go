package tools

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	odoolib "mcp-bedrock-go/odoo"
)

// Input: product_id (int) or mo_id (int)
// Output: JSON with BOM components and current stock levels
func MaterialAvailability(oclient *odoolib.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pid := req.GetInt("product_id", 0)
		moid := req.GetInt("mo_id", 0)

		var productID int
		if pid != 0 {
			productID = pid
		} else if moid != 0 {
			// fetch MO to get product
			mos, err := oclient.SearchRead("mrp.production", []string{"product_id", "product_qty"}, []any{[]any{"id", "=", moid}})
			if err == nil && len(mos) > 0 {
				if arr, ok := mos[0]["product_id"].([]any); ok && len(arr) > 0 {
					if v, ok := arr[0].(float64); ok {
						productID = int(v)
					}
				}
			}
		}

		if productID == 0 {
			return mcp.NewToolResultError("product_id or mo_id required"), nil
		}

		// fetch BOMs and stock quant (simplified)
		boms, _ := oclient.SearchRead("mrp.bom", []string{"id", "product_tmpl_id", "bom_line_ids"}, []any{[]any{"product_tmpl_id", "=", productID}})
		stock, _ := oclient.SearchRead("stock.quant", []string{"product_id", "quantity"}, []any{[]any{"product_id", "=", productID}})

		out := map[string]any{"product_id": productID, "boms": boms, "stock": stock}
		b, _ := json.MarshalIndent(out, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}
