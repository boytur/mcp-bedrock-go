package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/joho/godotenv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	bedrockruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

/* ============================================================
   BASIC STRUCTS (Odoo-Compatible)
============================================================ */

type WorkOrder struct {
	ID int `json:"id"`
}

type MRPOrder struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Product   string  `json:"product"`
	Qty       float64 `json:"qty"`
	Deadline  string  `json:"deadline"`
	State     string  `json:"state"`
	WorkOrder string  `json:"workorder_ids"`
}

/* ============================================================
   ODOO CLIENT (CLEAN, SAFE, UPDATED FOR ODOO 19)
============================================================ */

type OdooClient struct {
	url    string
	db     string
	user   string
	pass   string
	uid    int
	client *resty.Client
}

func NewOdooClient(url, db, user, pass string) *OdooClient {
	return &OdooClient{
		url: url, db: db, user: user, pass: pass,
		client: resty.New().SetTimeout(15 * time.Second),
	}
}

func (o *OdooClient) rpc(payload map[string]any, out any) ([]byte, error) {
	resp, err := o.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(o.url)

	if err != nil {
		return nil, err
	}

	body := resp.Body()

	if resp.IsError() {
		return body, fmt.Errorf("Odoo error HTTP %s: %s", resp.Status(), string(body))
	}

	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return body, fmt.Errorf("decode error: %v body=%s", err, string(body))
		}
	}
	return body, nil
}

func (o *OdooClient) Login() error {
	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  "call",
		"params": map[string]any{
			"service": "common",
			"method":  "authenticate",
			"args":    []any{o.db, o.user, o.pass, map[string]any{}},
		},
	}

	var resp map[string]any
	if _, err := o.rpc(req, &resp); err != nil {
		return err
	}

	if uid, ok := resp["result"].(float64); ok {
		o.uid = int(uid)
		return nil
	}

	return fmt.Errorf("unexpected login response: %v", resp)
}

func (o *OdooClient) searchRead(model string, fields []string, domain []any) ([]map[string]any, error) {

	if o.uid == 0 {
		return nil, fmt.Errorf("not authenticated")
	}

	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  "call",
		"params": map[string]any{
			"service": "object",
			"method":  "execute_kw",
			"args": []any{
				o.db, o.uid, o.pass,
				model, "search_read",
				[]any{domain},
				map[string]any{"fields": fields, "limit": 200},
			},
		},
	}

	var resp map[string]any
	body, err := o.rpc(req, &resp)
	if err != nil {
		return nil, fmt.Errorf("rpc failed: %v", err)
	}

	result, ok := resp["result"].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected search_read response (nil result). body=%s", string(body))
	}

	out := []map[string]any{}
	for _, r := range result {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}

	return out, nil
}

/* ============================================================
   TOOL: list_active_products
============================================================ */

func toolListActiveProducts(oclient *OdooClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {

		fields := []string{
			"id",
			"name",       // MO number
			"product_id", // [id, name]
			"product_qty",
			"date_deadline",
			"state",
			"workorder_ids", // [id1, id2...]
		}

		domain := []any{
			[]any{"state", "in", []string{"confirmed", "progress"}},
		}

		items, err := oclient.searchRead("mrp.production", fields, domain)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo error: %v", err)), nil
		}

		if len(items) == 0 {
			return mcp.NewToolResultText("No active manufacturing orders (confirmed or in progress)."), nil
		}

		out := []MRPOrder{}

		for _, it := range items {
			m := MRPOrder{}

			if v, ok := it["id"].(float64); ok {
				m.ID = int(v)
			}
			if v, ok := it["name"].(string); ok {
				m.Name = v
			}
			if arr, ok := it["product_id"].([]any); ok && len(arr) > 1 {
				m.Product = arr[1].(string)
			}
			if v, ok := it["product_qty"].(float64); ok {
				m.Qty = v
			}
			if v, ok := it["date_deadline"].(string); ok {
				m.Deadline = v
			}
			if v, ok := it["state"].(string); ok {
				m.State = v
			}
			if wos, ok := it["workorder_ids"].([]any); ok {
				m.WorkOrder = fmt.Sprintf("%v", wos)
			} else {
				m.WorkOrder = "-"
			}

			out = append(out, m)
		}

		jsonOut, _ := json.MarshalIndent(out, "", "  ")
		return mcp.NewToolResultText(string(jsonOut)), nil
	}
}

/* ============================================================
   TOOL: schedule_analysis (simple version)
============================================================ */

func toolScheduleAnalysis(oclient *OdooClient, br *bedrockruntime.Client, modelID string) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {

		profile, err := req.RequireString("profile")
		if err != nil {
			return mcp.NewToolResultError("Missing 'profile'"), nil
		}

		// Fetch live context from Odoo: active MOs and product list (small sample)
		moFields := []string{"id", "name", "product_id", "product_qty", "date_deadline", "state"}
		moDomain := []any{[]any{"state", "in", []string{"confirmed", "progress"}}}
		mos, err := oclient.searchRead("mrp.production", moFields, moDomain)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo fetch error: %v", err)), nil
		}

		type ctxMO struct {
			Name     string  `json:"name"`
			Product  string  `json:"product"`
			Qty      float64 `json:"qty"`
			Deadline string  `json:"deadline"`
			State    string  `json:"state"`
		}
		ctxMOs := []ctxMO{}
		for _, m := range mos {
			var item ctxMO
			if v, ok := m["name"].(string); ok {
				item.Name = v
			}
			if arr, ok := m["product_id"].([]any); ok && len(arr) > 1 {
				if s, ok := arr[1].(string); ok {
					item.Product = s
				}
			}
			if v, ok := m["product_qty"].(float64); ok {
				item.Qty = v
			}
			if v, ok := m["date_deadline"].(string); ok {
				item.Deadline = v
			}
			if v, ok := m["state"].(string); ok {
				item.State = v
			}
			ctxMOs = append(ctxMOs, item)
		}

		pFields := []string{"id", "default_code", "name"}
		pDomain := []any{[]any{}}
		products, _ := oclient.searchRead("product.product", pFields, pDomain)
		type ctxP struct {
			Code string `json:"code"`
			Name string `json:"name"`
		}
		ctxProducts := []ctxP{}
		for _, p := range products {
			var cp ctxP
			if arr, ok := p["default_code"].(string); ok {
				cp.Code = arr
			}
			if n, ok := p["name"].(string); ok {
				cp.Name = n
			}
			ctxProducts = append(ctxProducts, cp)
			if len(ctxProducts) >= 50 {
				break
			}
		}

		contextObject := map[string]any{
			"manufacturing_orders": ctxMOs,
			"products":             ctxProducts,
		}
		contextJSON, _ := json.MarshalIndent(contextObject, "", "  ")

		prompt := fmt.Sprintf("Profile: %s\n\nContext:\n%s\n\nTask: A new RUSH order (Product Code: RUSH-TEA, Qty:1000) is pending. Provide a single-paragraph recommendation and brief rationale based on the profile. If Cost-Aware, show net impact calculation.", profile, string(contextJSON))

		body, _ := json.Marshal(map[string]any{
			"prompt":      prompt,
			"max_gen_len": 400,
			"temperature": 0.3,
		})

		resp, err := br.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(modelID),
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
			Body:        body,
		})

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Bedrock error: %v", err)), nil
		}

		var out struct {
			Generation string `json:"generation"`
		}
		_ = json.Unmarshal(resp.Body, &out)
		if out.Generation == "" {
			return mcp.NewToolResultText(string(resp.Body)), nil
		}

		return mcp.NewToolResultText(out.Generation), nil
	}
}

/*
	===========================================================
	  Tool list all products

/*===========================================================
*/
func toolListAllOrders(oclient *OdooClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {

		fields := []string{
			"id",
			"name",
			"product_id",
			"product_qty",
			"date_deadline",
			"state",
			"workorder_ids",
		}

		domain := []any{
			[]any{"state", "!=", "done"},
		}

		items, err := oclient.searchRead("mrp.production", fields, domain)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Odoo error: %v", err)), nil
		}

		if len(items) == 0 {
			return mcp.NewToolResultText("No manufacturing orders found."), nil
		}

		out := []MRPOrder{}

		for _, it := range items {
			m := MRPOrder{}

			if v, ok := it["id"].(float64); ok {
				m.ID = int(v)
			}
			if v, ok := it["name"].(string); ok {
				m.Name = v
			}
			if arr, ok := it["product_id"].([]any); ok && len(arr) > 1 {
				m.Product = arr[1].(string)
			}
			if v, ok := it["product_qty"].(float64); ok {
				m.Qty = v
			}
			if v, ok := it["date_deadline"].(string); ok {
				m.Deadline = v
			}
			if v, ok := it["state"].(string); ok {
				m.State = v
			}
			if wos, ok := it["workorder_ids"].([]any); ok {
				m.WorkOrder = fmt.Sprintf("%v", wos)
			} else {
				m.WorkOrder = "-"
			}

			out = append(out, m)
		}

		jsonOut, _ := json.MarshalIndent(out, "", "  ")
		return mcp.NewToolResultText(string(jsonOut)), nil
	}
}

/* ============================================================
   MAIN — START MCP SERVER
============================================================ */

func main() {

	_ = godotenv.Load()

	// Load Odoo
	odoo := NewOdooClient(
		os.Getenv("ODOO_URL"),
		os.Getenv("ODOO_DB"),
		os.Getenv("ODOO_USERNAME"),
		os.Getenv("ODOO_API_KEY"),
	)

	if err := odoo.Login(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Odoo login failed: %v\n", err)
		os.Exit(1)
	}

	// AWS / Bedrock
	cfg, _ := config.LoadDefaultConfig(context.Background())
	br := bedrockruntime.NewFromConfig(cfg)
	modelID := "us.meta.llama3-1-70b-instruct-v1:0"

	// MCP server
	s := server.NewMCPServer(
		"bedrock-mcp",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	/* REGISTER TOOLS */
	s.AddTool(
		mcp.NewTool("list_active_products",
			mcp.WithDescription("List manufacturing orders currently in progress or confirmed."),
		),
		toolListActiveProducts(odoo),
	)

	s.AddTool(
		mcp.NewTool("schedule_analysis",
			mcp.WithDescription("Analyze scheduling impact of rush orders."),
			mcp.WithString("profile", mcp.Required()),
		),
		toolScheduleAnalysis(odoo, br, modelID),
	)

	s.AddTool(
		mcp.NewTool("list_all_orders",
			mcp.WithDescription("List ALL manufacturing orders except done."),
		),
		toolListAllOrders(odoo),
	)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Server error: %v\n", err)
	}
}
