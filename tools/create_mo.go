package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"

	odoolib "mcp-bedrock-go/odoo"
)

// CreateMO tool
// Input (strings):
// - `product_code` (default_code) OR `product_id` (string id)
// - `qty` (required) quantity to produce, string/number
// - `name` (optional) MO name
// - `date_deadline` (optional)
// Output: JSON {"mo_id": <id>, "message": "..."}
func CreateMO(oclient *odoolib.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Resolve inputs
		productCode := req.GetString("product_code", "")
		productIDStr := req.GetString("product_id", "")
		qtyStr := req.GetString("qty", "")
		name := req.GetString("name", "")
		dateDeadline := req.GetString("date_deadline", "")

		if qtyStr == "" {
			return mcp.NewToolResultError("'qty' is required"), nil
		}

		qty, err := strconv.ParseFloat(qtyStr, 64)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid qty: %v", err)), nil
		}

		// Find product
		var productSearchDomain []any
		if productIDStr != "" {
			// try direct id
			productSearchDomain = []any{[]any{"id", "=", productIDStr}}
		} else if productCode != "" {
			productSearchDomain = []any{[]any{"default_code", "=", productCode}}
		} else {
			return mcp.NewToolResultError("product_code or product_id is required"), nil
		}

		prodFields := []string{"id", "product_tmpl_id", "default_code", "name"}
		prods, err := oclient.SearchRead("product.product", prodFields, productSearchDomain)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo error finding product: %v", err)), nil
		}
		if len(prods) == 0 {
			return mcp.NewToolResultError("Product not found. Create product first or check code."), nil
		}

		// take first product
		prod := prods[0]
		pidAny, ok := prod["id"]
		if !ok {
			return mcp.NewToolResultError("product id missing in response"), nil
		}
		pid := 0
		switch v := pidAny.(type) {
		case float64:
			pid = int(v)
		case int:
			pid = v
		default:
			return mcp.NewToolResultError("unexpected product id type"), nil
		}

		// Check BOM exists for this product (mrp.bom product_tmpl_id)
		// product may reference product_tmpl_id as [id, name]
		tmplID := 0
		if pt, ok := prod["product_tmpl_id"]; ok {
			switch x := pt.(type) {
			case []any:
				if len(x) > 0 {
					if f, ok := x[0].(float64); ok {
						tmplID = int(f)
					}
				}
			case float64:
				tmplID = int(x)
			case int:
				tmplID = x
			}
		}

		bomDomain := []any{}
		if tmplID != 0 {
			bomDomain = []any{[]any{"product_tmpl_id", "=", tmplID}}
		} else {
			bomDomain = []any{[]any{"product_tmpl_id", "=", pid}}
		}
		boms, _ := oclient.SearchRead("mrp.bom", []string{"id", "product_tmpl_id"}, bomDomain)
		if len(boms) == 0 {
			return mcp.NewToolResultError("No BOM found for product. Create BOM before creating MO."), nil
		}

		// build create vals for mrp.production
		vals := map[string]any{
			"product_id":  pid,
			"product_qty": qty,
		}
		if name != "" {
			vals["name"] = name
		}
		if dateDeadline != "" {
			vals["date_planned_start"] = dateDeadline
		}

		moID, err := oclient.Create("mrp.production", vals)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo create MO error: %v", err)), nil
		}

		resp := map[string]any{"mo_id": moID, "message": "Manufacturing Order created"}
		b, _ := json.MarshalIndent(resp, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}
