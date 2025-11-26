// Tool: ListProductMeta
// คำอธิบาย (ไทย): ดึงข้อมูล metadata ที่เกี่ยวกับสินค้า เช่น หมวดหมู่, หน่วยวัด, แม่แบบสินค้า และแอตทริบิวต์ เพื่อช่วยสร้างสินค้าหรือ UI
package tools

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	odoolib "mcp-bedrock-go/odoo"
)

// ListProductMeta returns common product metadata useful for creating products
// Input: optional `filter` (string) to search by name
// Output: JSON object { categories: [], uoms: [], attributes: [], templates: [] }
func ListProductMeta(oclient *odoolib.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filter := req.GetString("filter", "")

		// categories
		catFields := []string{"id", "name", "parent_id"}
		var catDomain []any
		if filter != "" {
			catDomain = []any{[]any{"name", "ilike", filter}}
		} else {
			catDomain = []any{}
		}
		cats, _ := oclient.SearchRead("product.category", catFields, catDomain)

		// uoms
		uomFields := []string{"id", "name", "factor", "category_id"}
		var uomDomain []any
		if filter != "" {
			uomDomain = []any{[]any{"name", "ilike", filter}}
		} else {
			uomDomain = []any{}
		}
		uoms, _ := oclient.SearchRead("uom.uom", uomFields, uomDomain)

		// product attributes
		attrFields := []string{"id", "name"}
		attrs, _ := oclient.SearchRead("product.attribute", attrFields, []any{})
		// attribute values
		avFields := []string{"id", "name", "attribute_id"}
		avs, _ := oclient.SearchRead("product.attribute.value", avFields, []any{})

		// product templates (key summary)
		tmplFields := []string{"id", "name", "default_code"}
		var tmplDomain []any
		if filter != "" {
			tmplDomain = []any{[]any{"name", "ilike", filter}}
		} else {
			tmplDomain = []any{}
		}
		tmpls, _ := oclient.SearchRead("product.template", tmplFields, tmplDomain)

		// build attribute map of values
		attrMap := map[int][]map[string]any{}
		for _, v := range avs {
			if aid, ok := toInt(v["attribute_id"]); ok {
				attrMap[aid] = append(attrMap[aid], v)
			}
		}

		out := map[string]any{
			"categories":       cats,
			"uoms":             uoms,
			"attributes":       attrs,
			"attribute_values": attrMap,
			"templates":        tmpls,
		}

		b, _ := json.MarshalIndent(out, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case []any:
		if len(x) > 0 {
			if f, ok := x[0].(float64); ok {
				return int(f), true
			}
		}
	case float64:
		return int(x), true
	case int:
		return x, true
	}
	return 0, false
}
