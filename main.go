package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"github.com/aws/aws-sdk-go-v2/config"
	bedrockruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	bedrocklib "mcp-bedrock-go/bedrock"
	odoolib "mcp-bedrock-go/odoo"
	tools "mcp-bedrock-go/tools"
)

func main() {
	_ = godotenv.Load()

	// Init Odoo
	odoo := odoolib.New(
		os.Getenv("ODOO_URL"),
		os.Getenv("ODOO_DB"),
		os.Getenv("ODOO_USERNAME"),
		os.Getenv("ODOO_API_KEY"),
	)
	if err := odoo.Login(); err != nil {
		log.Fatalf("Odoo login failed: %v", err)
	}

	// Init AWS Bedrock
	cfg, _ := config.LoadDefaultConfig(context.Background())
	brInner := bedrockruntime.NewFromConfig(cfg)
	br := bedrocklib.New(brInner)

	modelID := "us.meta.llama3-1-70b-instruct-v1:0"

	// MCP Server
	s := server.NewMCPServer(
		"bedrock-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	// Register Tools
	s.AddTool(
		mcp.NewTool("list_all_orders", mcp.WithDescription("List all manufacturing orders")),
		tools.ListAllOrders(odoo),
	)

	s.AddTool(
		mcp.NewTool("list_active_products", mcp.WithDescription("List active manufacturing orders")),
		tools.ListActiveProducts(odoo),
	)

	s.AddTool(
		mcp.NewTool("schedule_analysis",
			mcp.WithDescription("Analyze scheduling impact"),
			mcp.WithString("profile", mcp.Required())),
		tools.ScheduleAnalysis(odoo, br, modelID),
	)

	s.AddTool(
		mcp.NewTool("capacity_check",
			mcp.WithDescription("Check capacity"),
			mcp.WithString("workcenter_id", mcp.Required()),
			mcp.WithString("date", mcp.Required())),
		tools.CapacityCheck(odoo),
	)

	s.AddTool(
		mcp.NewTool("order_priority", mcp.WithDescription("Rank MOs")),
		tools.OrderPriority(odoo),
	)

	s.AddTool(
		mcp.NewTool("order_risk",
			mcp.WithDescription("Risk assessment"),
			mcp.WithString("mo_id", mcp.Required())),
		tools.OrderRisk(odoo),
	)

	s.AddTool(
		mcp.NewTool("material_availability",
			mcp.WithDescription("Check BOM/stock"),
			mcp.WithString("product_id", mcp.Required()),
			mcp.WithString("mo_id", mcp.Required())),
		tools.MaterialAvailability(odoo),
	)

	s.AddTool(
		mcp.NewTool("add_product",
			mcp.WithDescription("Add product"),
			mcp.WithString("name", mcp.Required()),
			mcp.WithString("default_code"),
			mcp.WithString("type"),
			mcp.WithString("list_price")),
		tools.AddProduct(odoo),
	)

	s.AddTool(
		mcp.NewTool("list_product_meta", mcp.WithDescription("List product metadata")),
		tools.ListProductMeta(odoo),
	)

	s.AddTool(
		mcp.NewTool("create_mo",
			mcp.WithDescription("Create manufacturing order"),
			mcp.WithString("product_code"),
			mcp.WithString("product_id"),
			mcp.WithString("qty", mcp.Required()),
			mcp.WithString("name"),
			mcp.WithString("date_deadline")),
		tools.CreateMO(odoo),
	)

	// Run STDIO (for IDE)
	go func() {
		if err := server.ServeStdio(s); err != nil {
			log.Fatalf("STDIO server error: %v", err)
		}
	}()

	// Create SSE HTTP Transport and mount endpoints
	sse := server.NewSSEServer(s)

	// Mount MCP SSE endpoints
	http.Handle("/sse", sse.SSEHandler())
	http.Handle("/message", sse.MessageHandler())

	// Optionally keep your own HTTP API
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"server": "bedrock-mcp",
		})
	})

	log.Println("MCP SSE HTTP server running on http://localhost:5982")
	log.Fatal(http.ListenAndServe(":5982", nil))
}
