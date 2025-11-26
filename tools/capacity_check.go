// Tool: CapacityCheck
// คำอธิบาย (ไทย): ตรวจสอบความสามารถของ workcenter ภายในวันที่ที่ระบุ โดยคืนสรุปงาน (workorders) และการใช้งานเป็น JSON
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	odoolib "mcp-bedrock-go/odoo"
)

// Input: workcenter_id (int, optional), date (string, optional, YYYY-MM-DD)
// Output: JSON summary of capacity usage
func CapacityCheck(oclient *odoolib.Client) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wcID := req.GetInt("workcenter_id", 0)
		dateStr := req.GetString("date", "")
		if dateStr == "" {
			dateStr = time.Now().Format("2006-01-02")
		}

		// Simplified: fetch workorders scheduled on that date
		fields := []string{"id", "name", "workcenter_id", "state", "date_planned_start", "date_planned_finished", "duration"}
		domain := []any{[]any{"date_planned_start", ">=", dateStr}, []any{"date_planned_start", "<=", dateStr}}
		if wcID != 0 {
			domain = append(domain, []any{"workcenter_id", "=", wcID})
		}

		items, err := oclient.SearchRead("mrp.workorder", fields, domain)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo error: %v", err)), nil
		}

		b, _ := json.MarshalIndent(map[string]any{"date": dateStr, "workorders": items}, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	}
}
