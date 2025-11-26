package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/go-resty/resty/v2"
	"github.com/joho/godotenv"
)

// ======================================================
// =============== CONFIG VARIABLES =====================
// ======================================================

var (
	odooURL  string
	db       string
	username string
	apiKey   string
	client   = resty.New()
	uid      int
)

// ======================================================
// ================= JSON-RPC TYPES =====================
// ======================================================

type RPCErrorData struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

type RPCError struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    RPCErrorData `json:"data"`
}

type RPCResponse[T any] struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      int       `json:"id"`
	Result  *T        `json:"result"`
	Error   *RPCError `json:"error"`
}

type RPCRequest struct {
	JSONRPC string              `json:"jsonrpc"`
	Method  string              `json:"method"`
	Params  RPCRequestParamsObj `json:"params"`
	ID      int                 `json:"id"`
}

type RPCRequestParamsObj struct {
	Service string        `json:"service"`
	Method  string        `json:"method"`
	Args    []interface{} `json:"args"`
}

// ======================================================
// ================= MOCK JSON STRUCTS ==================
// ======================================================

type MockData struct {
	Workcenters []Workcenter  `json:"workcenters"`
	Products    []MockProduct `json:"products"`
	BOM         []BOMItem     `json:"bom"`
	Routing     []RoutingItem `json:"routing"`
	MRPOrders   []MRPOrder    `json:"mrp_orders"`
}

type Workcenter struct {
	Name           string  `json:"name"`
	Capacity       int     `json:"capacity"`
	CostPerHour    float64 `json:"cost_per_hour"`
	TimeEfficiency float64 `json:"time_efficiency"`
}

type MockProduct struct {
	Name             string  `json:"name"`
	DefaultCode      string  `json:"default_code"`
	Type             string  `json:"type"` // ไม่ได้ใช้ส่งเข้า Odoo แล้ว แต่เผื่อใช้ต่อ
	ListPrice        float64 `json:"list_price"`
	StandardPrice    float64 `json:"standard_price"`
	TardinessPenalty float64 `json:"tardiness_penalty"`
}

type BOMItem struct {
	ProductDefaultCode string    `json:"product_default_code"`
	Lines              []BOMLine `json:"lines"`
}

type BOMLine struct {
	Product string  `json:"product"`
	Qty     float64 `json:"qty"`
}

type RoutingItem struct {
	Product        string   `json:"product"`
	Steps          []string `json:"steps"`
	OperationTimes []int    `json:"operation_times"`
}

type MRPOrder struct {
	ProductDefaultCode string  `json:"product_default_code"`
	Qty                float64 `json:"qty"`
	Deadline           string  `json:"deadline"`
	Status             string  `json:"status"`
	CurrentStep        string  `json:"current_step"`
	RemainingMins      float64 `json:"remaining_time_mins"`
	Rush               bool    `json:"rush"`
	OTCost             float64 `json:"ot_cost"`
}

// ======================================================
// ================== UTIL FUNCTIONS ====================
// ======================================================

func anyToInt(v interface{}) (int, error) {
	switch t := v.(type) {
	case float64:
		return int(t), nil
	case int:
		return t, nil
	case string:
		return strconv.Atoi(t)
	case json.Number:
		i, err := t.Int64()
		return int(i), err
	default:
		return 0, fmt.Errorf("invalid type: %T", v)
	}
}

func extractMany2oneID(v interface{}) (int, error) {
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return 0, errors.New("invalid many2one value")
	}
	return anyToInt(arr[0])
}

func callRPC[T any](req RPCRequest) (*T, error) {
	var resp RPCResponse[T]

	_, err := client.R().
		SetBody(req).
		SetResult(&resp).
		Post(odooURL)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error: %s - %s",
			resp.Error.Message, resp.Error.Data.Message)
	}

	if resp.Result == nil {
		return nil, errors.New("rpc result is nil")
	}

	return resp.Result, nil
}

// ======================================================
// ======================== LOGIN =======================
// ======================================================

func login() {
	req := RPCRequest{
		JSONRPC: "2.0",
		Method:  "call",
		Params: RPCRequestParamsObj{
			Service: "common",
			Method:  "authenticate",
			Args:    []interface{}{db, username, apiKey, map[string]interface{}{}},
		},
		ID: 1,
	}

	result, err := callRPC[int](req)
	if err != nil {
		log.Fatalf("Login failed: %v", err)
	}

	uid = *result
	fmt.Println("Logged in → UID =", uid)
}

// ======================================================
// =========== IMPORT PRODUCTS (template+variant) =======
// ======================================================

func importProducts(mock MockData) (map[string]int, map[string]int) {
	templateIDs := map[string]int{}
	variantIDs := map[string]int{}

	for _, p := range mock.Products {
		// 1) Create product.template (ไม่ส่ง type/detailed_type แล้ว)
		templateBody := map[string]interface{}{
			"name":           p.Name,
			"default_code":   p.DefaultCode,
			"list_price":     p.ListPrice,
			"standard_price": p.StandardPrice,
		}

		createTemplateReq := RPCRequest{
			JSONRPC: "2.0",
			Method:  "call",
			Params: RPCRequestParamsObj{
				Service: "object",
				Method:  "execute_kw",
				Args:    []interface{}{db, uid, apiKey, "product.template", "create", []interface{}{templateBody}},
			},
			ID: 2,
		}

		templateID, err := callRPC[int](createTemplateReq)
		if err != nil {
			log.Fatalf("template create error: %v", err)
		}
		templateIDs[p.DefaultCode] = *templateID

		// 2) อ่าน variant ที่ Odoo สร้างให้ (product_variant_id)
		readReq := RPCRequest{
			JSONRPC: "2.0",
			Method:  "call",
			Params: RPCRequestParamsObj{
				Service: "object",
				Method:  "execute_kw",
				Args: []interface{}{
					db, uid, apiKey,
					"product.template", "read",
					[]interface{}{[]int{*templateID}},
					map[string]interface{}{"fields": []string{"product_variant_id"}},
				},
			},
			ID: 3,
		}

		type ReadResult = []map[string]interface{}
		readRes, err := callRPC[ReadResult](readReq)
		if err != nil {
			log.Fatalf("product_variant read error: %v", err)
		}

		variantID, err := extractMany2oneID((*readRes)[0]["product_variant_id"])
		if err != nil {
			log.Fatalf("variant id parse error: %v", err)
		}

		variantIDs[p.DefaultCode] = variantID

		fmt.Println("Created Product:", p.DefaultCode, "→ template_id:", *templateID, "variant_id:", variantID)
	}

	return templateIDs, variantIDs
}

// ======================================================
// ======================== BOM =========================
// ======================================================

func importBOM(mock MockData, templateIDs, variantIDs map[string]int) {
	for _, b := range mock.BOM {
		tmplID, ok := templateIDs[b.ProductDefaultCode]
		if !ok {
			log.Fatalf("no template id for product_default_code=%s", b.ProductDefaultCode)
		}

		bomBody := map[string]interface{}{
			"product_tmpl_id": tmplID,
		}

		var lines []interface{}
		for _, l := range b.Lines {
			vID, ok := variantIDs[l.Product]
			if !ok {
				log.Fatalf("no variant id for BOM component=%s", l.Product)
			}
			lines = append(lines, []interface{}{
				0, 0,
				map[string]interface{}{
					"product_id":  vID,
					"product_qty": l.Qty,
				},
			})
		}
		bomBody["bom_line_ids"] = lines

		req := RPCRequest{
			JSONRPC: "2.0",
			Method:  "call",
			Params: RPCRequestParamsObj{
				Service: "object",
				Method:  "execute_kw",
				Args:    []interface{}{db, uid, apiKey, "mrp.bom", "create", []interface{}{bomBody}},
			},
			ID: 4,
		}

		_, err := callRPC[int](req)
		if err != nil {
			log.Fatalf("BOM create error: %v", err)
		}

		fmt.Println("BOM created →", b.ProductDefaultCode)
	}
}

// ======================================================
// ========================= MRP ========================
// ======================================================

func importMRP(mock MockData, variantIDs map[string]int) {
	for _, mo := range mock.MRPOrders {
		vID, ok := variantIDs[mo.ProductDefaultCode]
		if !ok {
			log.Fatalf("no variant id for mrp product_default_code=%s", mo.ProductDefaultCode)
		}

		body := map[string]interface{}{
			"product_id":    vID,
			"product_qty":   mo.Qty,
			"date_deadline": mo.Deadline,
		}

		req := RPCRequest{
			JSONRPC: "2.0",
			Method:  "call",
			Params: RPCRequestParamsObj{
				Service: "object",
				Method:  "execute_kw",
				Args:    []interface{}{db, uid, apiKey, "mrp.production", "create", []interface{}{body}},
			},
			ID: 5,
		}

		_, err := callRPC[int](req)
		if err != nil {
			log.Fatalf("MRP create error: %v", err)
		}

		fmt.Println("MRP created →", mo.ProductDefaultCode)
	}
}

// ======================================================
// ========================== MAIN ======================
// ======================================================

func main() {
	_ = godotenv.Load()

	odooURL = os.Getenv("ODOO_URL")
	db = os.Getenv("ODOO_DB")
	username = os.Getenv("ODOO_USERNAME")
	apiKey = os.Getenv("ODOO_API_KEY")

	login()

	dataBytes, err := os.ReadFile("mock.json")
	if err != nil {
		log.Fatalf("Error reading mock.json: %v", err)
	}

	var mock MockData
	if err := json.Unmarshal(dataBytes, &mock); err != nil {
		log.Fatalf("Error unmarshalling mock.json: %v", err)
	}

	templateIDs, variantIDs := importProducts(mock)
	importBOM(mock, templateIDs, variantIDs)
	importMRP(mock, variantIDs)

	fmt.Println("✨ Import Completed")
}
