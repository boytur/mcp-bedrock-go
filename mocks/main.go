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
// ===============  CONFIG VARIABLES  ====================
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
// ================== JSON STRUCTS =======================
// ======================================================

// ==== Generic JSON-RPC Structs ====

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
// ================ MOCK JSON STRUCTS ====================
// ======================================================

type MockData struct {
	Workcenters []ProductObject `json:"workcenters"`
	Products    []ProductObject `json:"products"`
	BOM         []BOMItem       `json:"bom"`
	Routing     []RoutingItem   `json:"routing"`
	MRPOrders   []MRPOrder      `json:"mrp_orders"`
}

type ProductObject struct {
	DefaultCode string            `json:"default_code"`
	Raw         map[string]string `json:"-"`
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
	Product string   `json:"product"`
	Steps   []string `json:"steps"`
}

type MRPOrder struct {
	ProductDefaultCode string  `json:"product_default_code"`
	Qty                float64 `json:"qty"`
	Deadline           string  `json:"deadline"`
}

// ======================================================
// ================= UTILITY FUNCTIONS ==================
// ======================================================

func anyToInt(v interface{}) (int, error) {
	switch t := v.(type) {
	case float64:
		return int(t), nil
	case float32:
		return int(t), nil
	case int:
		return t, nil
	case int64:
		return int(t), nil
	case json.Number:
		i, err := t.Int64()
		return int(i), err
	case string:
		return strconv.Atoi(t)
	default:
		return 0, fmt.Errorf("invalid numeric type: %T", v)
	}
}

func extractTemplateID(v interface{}) (int, error) {
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return 0, errors.New("invalid template array")
	}

	return anyToInt(arr[0])
}

// ======================================================
// =================== JSON-RPC CALLER ===================
// ======================================================

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
		return nil, fmt.Errorf("rpc error: %s (%s)", resp.Error.Message, resp.Error.Data.Message)
	}

	if resp.Result == nil {
		return nil, errors.New("rpc result is nil")
	}

	return resp.Result, nil
}

// ======================================================
// ========================= LOGIN =======================
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
		log.Fatalf("login failed: %v", err)
	}

	uid = *result
	fmt.Println("Logged in → UID =", uid)
}

// ======================================================
// =============== IMPORT PRODUCTS ========================
// ======================================================

func importProducts(mock MockData) map[string]int {
	idMap := map[string]int{}

	for _, p := range mock.Products {
		body := map[string]interface{}{
			"default_code": p.DefaultCode,
		}

		req := RPCRequest{
			JSONRPC: "2.0",
			Method:  "call",
			Params: RPCRequestParamsObj{
				Service: "object",
				Method:  "execute_kw",
				Args:    []interface{}{db, uid, apiKey, "product.product", "create", []interface{}{body}},
			},
			ID: 1,
		}

		result, err := callRPC[float64](req)
		if err != nil {
			log.Fatalf("product create error: %v", err)
		}

		id := int(*result)
		idMap[p.DefaultCode] = id

		fmt.Println("Created Product:", p.DefaultCode, "→", id)
	}

	return idMap
}

// ======================================================
// ===================== IMPORT BOM ======================
// ======================================================

func importBOM(mock MockData, idMap map[string]int) {
	for _, b := range mock.BOM {

		// Read product_tmpl_id
		reqRead := RPCRequest{
			JSONRPC: "2.0",
			Method:  "call",
			Params: RPCRequestParamsObj{
				Service: "object",
				Method:  "execute_kw",
				Args: []interface{}{
					db, uid, apiKey,
					"product.product", "read",
					[]interface{}{[]int{idMap[b.ProductDefaultCode]}},
					map[string]interface{}{"fields": []string{"product_tmpl_id"}},
				},
			},
			ID: 1,
		}

		type ReadResult = []map[string]interface{}

		readRes, err := callRPC[ReadResult](reqRead)
		if err != nil {
			log.Fatalf("read template error: %v", err)
		}

		templateID, err := extractTemplateID((*readRes)[0]["product_tmpl_id"])
		if err != nil {
			log.Fatalf("template id parse error: %v", err)
		}

		var lineItems []interface{}
		for _, ln := range b.Lines {
			lineItems = append(lineItems, []interface{}{
				0, 0, map[string]interface{}{
					"product_id":  idMap[ln.Product],
					"product_qty": ln.Qty,
				},
			})
		}

		bomBody := map[string]interface{}{
			"product_tmpl_id": templateID,
			"bom_line_ids":    lineItems,
		}

		reqCreate := RPCRequest{
			JSONRPC: "2.0",
			Method:  "call",
			Params: RPCRequestParamsObj{
				Service: "object",
				Method:  "execute_kw",
				Args:    []interface{}{db, uid, apiKey, "mrp.bom", "create", []interface{}{bomBody}},
			},
			ID: 1,
		}

		_, err = callRPC[int](reqCreate)
		if err != nil {
			log.Fatalf("bom create error: %v", err)
		}

		fmt.Println("BOM Created for:", b.ProductDefaultCode)
	}
}

// ======================================================
// ================== IMPORT MRP ==========================
// ======================================================

func importMRP(mock MockData, idMap map[string]int) {
	for _, mo := range mock.MRPOrders {
		body := map[string]interface{}{
			"product_id":    idMap[mo.ProductDefaultCode],
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
			ID: 1,
		}

		_, err := callRPC[int](req)
		if err != nil {
			log.Fatalf("mrp create error: %v", err)
		}

		fmt.Println("MRP Created:", mo.ProductDefaultCode)
	}
}

// ======================================================
// ========================== MAIN =======================
// ======================================================

func main() {
	_ = godotenv.Load()

	odooURL = os.Getenv("ODOO_URL")
	db = os.Getenv("ODOO_DB")
	username = os.Getenv("ODOO_USERNAME")
	apiKey = os.Getenv("ODOO_API_KEY")

	login()

	dataBytes, _ := os.ReadFile("mock.json")

	var mock MockData
	json.Unmarshal(dataBytes, &mock)

	idMap := importProducts(mock)
	importBOM(mock, idMap)
	importMRP(mock, idMap)

	fmt.Println("✨ Import Completed")
}
