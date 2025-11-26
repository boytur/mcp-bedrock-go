package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	odoolib "mcp-bedrock-go/odoo"
)

// Input schema (strings accepted for simplicity):
// - `name` (required) product name
// - `default_code` (optional) product code/SKU
// - `type` (optional) "product" or "service"
// - `list_price` (optional) string numeric price
// Output: JSON {"id": <created_id>} or friendly error
func AddProduct(oclient *odoolib.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("'name' is required"), nil
		}
		code := req.GetString("default_code", "")
		ptype := req.GetString("type", "product")
		price := req.GetString("list_price", "0")

		vals := map[string]any{"name": name, "default_code": code, "type": ptype}
		// Try to parse price into float if provided
		if price != "" && price != "0" {
			vals["list_price"] = price
		}

		id, err := oclient.Create("product.product", vals)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo create error: %v", err)), nil
		}

		resp := map[string]any{"id": id}
		b, _ := json.MarshalIndent(resp, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}
