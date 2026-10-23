package httpapi

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"tradestocker/internal/contracts"
	"tradestocker/internal/paper"
)

const openAPI = "openapi/local-api.v1.json"

func apiRegistry(t *testing.T) *contracts.Registry {
	t.Helper()
	r, err := contracts.NewRegistry(os.DirFS("../../contracts"), openAPI,
		"research/sec-annual-v2.report.schema.json", "research/sec-annual-v2.request.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type apiOperation struct {
	method, path, id string
	node             map[string]any
}

func operations(t *testing.T, r *contracts.Registry) []apiOperation {
	t.Helper()
	doc, _ := r.Document(openAPI)
	var out []apiOperation
	for p, item := range doc.(map[string]any)["paths"].(map[string]any) {
		for method, o := range item.(map[string]any) {
			node := o.(map[string]any)
			out = append(out, apiOperation{strings.ToUpper(method), p, node["operationId"].(string), node})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].method+out[i].path < out[j].method+out[j].path })
	return out
}

func pointerPart(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// schemaRefs lists every schema location in the OpenAPI document: each
// component schema and each inline parameter, body, response and header schema.
func schemaRefs(t *testing.T, r *contracts.Registry) []string {
	t.Helper()
	doc, _ := r.Document(openAPI)
	d := doc.(map[string]any)
	var refs []string
	for name := range d["components"].(map[string]any)["schemas"].(map[string]any) {
		refs = append(refs, openAPI+"#/components/schemas/"+pointerPart(name))
	}
	for name := range d["components"].(map[string]any)["headers"].(map[string]any) {
		refs = append(refs, openAPI+"#/components/headers/"+pointerPart(name)+"/schema")
	}
	refs = append(refs, openAPI+"#/components/parameters/IdempotencyKey/schema")
	for _, op := range operations(t, r) {
		base := openAPI + "#/paths/" + pointerPart(op.path) + "/" + strings.ToLower(op.method)
		if params, ok := op.node["parameters"].([]any); ok {
			for i, p := range params {
				if _, inline := p.(map[string]any)["schema"]; inline {
					refs = append(refs, base+"/parameters/"+strconv.Itoa(i)+"/schema")
				}
			}
		}
		if _, ok := op.node["requestBody"]; ok {
			refs = append(refs, base+"/requestBody/content/application~1json/schema")
		}
		for status, resp := range op.node["responses"].(map[string]any) {
			if _, ok := resp.(map[string]any)["content"]; ok {
				refs = append(refs, base+"/responses/"+status+"/content/application~1json/schema")
			}
		}
	}
	sort.Strings(refs)
	return refs
}

func TestOpenAPIDocumentIsComplete(t *testing.T) {
	r := apiRegistry(t)
	doc, _ := r.Document(openAPI)
	d := doc.(map[string]any)
	if d["openapi"] != "3.1.0" || d["jsonSchemaDialect"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("openapi %v dialect %v", d["openapi"], d["jsonSchemaDialect"])
	}
	for _, ref := range schemaRefs(t, r) {
		if problems := r.CheckDialect(ref, contracts.APIProfile); len(problems) > 0 {
			t.Errorf("%s:\n  %s", ref, strings.Join(problems, "\n  "))
		}
	}
	ids := map[string]bool{}
	for _, op := range operations(t, r) {
		if ids[op.id] {
			t.Errorf("duplicate operationId %s", op.id)
		}
		ids[op.id] = true
		for status, resp := range op.node["responses"].(map[string]any) {
			if _, err := strconv.Atoi(status); err != nil {
				t.Errorf("%s: response key %q is not a status code", op.id, status)
			}
			if desc, _ := resp.(map[string]any)["description"].(string); desc == "" {
				t.Errorf("%s %s: response without a description", op.id, status)
			}
		}
		// Only the unauthenticated routes override the document's owner-token security.
		sec, overridden := op.node["security"].([]any)
		public := op.path == "/api/v1/live" || op.path == "/api/v1/ready" || op.path == "/api/v1/health"
		if public != (overridden && len(sec) == 0) {
			t.Errorf("%s: security does not match the server (public %v)", op.id, public)
		}
		if op.method == "POST" && strings.HasPrefix(op.path, "/api/v1/") {
			if _, ok := op.node["x-tradestocker-idempotency"].(string); !ok {
				t.Errorf("%s: POST without its idempotency rule", op.id)
			}
		}
	}
}

// routesInSource reads every route pattern registered with mux.Handle or
// mux.HandleFunc in the given Go files.
func routesInSource(t *testing.T, files ...string) map[string]bool {
	t.Helper()
	routes := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "HandleFunc" && sel.Sel.Name != "Handle") {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Errorf("%s: route pattern is not a literal", fset.Position(call.Pos()))
				return true
			}
			pattern, _ := strconv.Unquote(lit.Value)
			if strings.Contains(pattern, " ") {
				routes[pattern] = true
			} else if pattern != "/api/v1/research/" {
				t.Errorf("unexpected method-less route %q", pattern)
			}
			return true
		})
	}
	return routes
}

// Reconciliation: the routes the server registers are exactly the documented
// operations (the research mux is mounted under /api/v1/research/).
func TestDocumentedRoutesAreExactlyTheServedRoutes(t *testing.T) {
	r := apiRegistry(t)
	served := routesInSource(t, "server.go", filepath.Join("..", "research", "service.go"))
	documented := map[string]bool{}
	for _, op := range operations(t, r) {
		documented[op.method+" "+op.path] = true
	}
	for route := range served {
		if !documented[route] {
			t.Errorf("served but not documented: %s", route)
		}
	}
	for route := range documented {
		if !served[route] {
			t.Errorf("documented but not served: %s", route)
		}
	}
	if len(served) != 19 {
		t.Fatalf("%d served routes", len(served))
	}
}

// INV-01: no order submission, trade execution or broker endpoint exists, in
// the contract or at runtime. Fill and sell record past human paper executions.
func TestNoOrderSubmissionEndpointExists(t *testing.T) {
	r := apiRegistry(t)
	forbidden := regexp.MustCompile(`(?i)order|execut|broker|trade|submit|place|liquidat`)
	for _, op := range operations(t, r) {
		if forbidden.MatchString(op.path) || forbidden.MatchString(op.id) {
			t.Errorf("order-like apiOperation %s %s (%s)", op.method, op.path, op.id)
		}
		if strings.HasSuffix(op.path, "/fill") || strings.HasSuffix(op.path, "/sell") {
			if _, ok := op.node["x-tradestocker-execution-report"]; !ok {
				t.Errorf("%s records an execution but is not marked as an execution report", op.id)
			}
		}
	}
	h := (&API{Store: &fakeStore{state: paper.NewState()}, Token: strings.Repeat("a", 32)}).Handler()
	for _, p := range []string{"/api/v1/orders", "/api/v1/execute-trade", "/api/v1/broker/orders", "/api/v1/paper/orders",
		"/api/v1/accounts/x/orders", "/api/v1/accounts/x/execution-reports", "/api/v1/trades"} {
		req := httptest.NewRequest(http.MethodPost, p, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "no-orders-1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("POST %s answered %d, want 404", p, w.Code)
		}
	}
}

// responseSchema returns the documented schema for one answer of op.
func responseSchema(t *testing.T, op apiOperation, status int) string {
	t.Helper()
	resp, ok := op.node["responses"].(map[string]any)[strconv.Itoa(status)].(map[string]any)
	if !ok {
		t.Fatalf("%s answered %d, which is not documented", op.id, status)
	}
	if _, ok := resp["content"]; !ok {
		t.Fatalf("%s %d has no documented body", op.id, status)
	}
	return openAPI + "#/paths/" + pointerPart(op.path) + "/" + strings.ToLower(op.method) + "/responses/" + strconv.Itoa(status) + "/content/application~1json/schema"
}

type conformance struct {
	t       *testing.T
	r       *contracts.Registry
	h       http.Handler
	ops     map[string]apiOperation
	token   string
	n       int
	capture map[string][]byte
	last    []byte
}

// call sends one real request through the real handler and checks the answer
// against the apiOperation's documented schema for that status code.
func (c *conformance) call(opID, path, body string, want int, headers map[string]string) map[string]any {
	c.t.Helper()
	op := c.ops[opID]
	req := httptest.NewRequest(op.method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
		c.n++
		req.Header.Set("Idempotency-Key", "contract-"+strconv.Itoa(c.n))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	if w.Code != want {
		c.t.Fatalf("%s %s: status %d, want %d: %s", op.method, path, w.Code, want, w.Body)
	}
	v, err := contracts.Parse(w.Body.Bytes())
	if err != nil {
		c.t.Fatalf("%s: response is not strict JSON: %v", opID, err)
	}
	if violation := c.r.Validate(responseSchema(c.t, op, w.Code), v); violation != nil {
		c.t.Fatalf("%s %d breaks its contract: %v\n%s", opID, w.Code, violation, w.Body)
	}
	if w.Header().Get("traceparent") == "" || w.Header().Get("X-Trace-Id") == "" {
		c.t.Fatalf("%s: trace headers missing", opID)
	}
	c.last = append([]byte(nil), w.Body.Bytes()...)
	if key := opID + "." + strconv.Itoa(w.Code); c.capture[key] == nil {
		c.capture[key] = c.last // the first answer of each kind is the golden fixture
	}
	m, _ := v.(map[string]any)
	return m
}

// Every paper and health answer the real handler gives, for success and for
// each error family, matches the documented schema. Captured answers become
// the API golden fixtures (UPDATE_CONTRACT_FIXTURES=1 rewrites them).
func TestServedAnswersMatchTheContract(t *testing.T) {
	r := apiRegistry(t)
	ops := map[string]apiOperation{}
	for _, op := range operations(t, r) {
		ops[op.id] = op
	}
	token := strings.Repeat("t", 32)
	c := &conformance{t: t, r: r, ops: ops, token: token, capture: map[string][]byte{},
		h: (&API{Store: &fakeStore{state: paper.NewState()}, Token: token}).Handler()}
	c.call("getLive", "/api/v1/live", "", 200, nil)
	c.call("getReady", "/api/v1/ready", "", 200, nil)
	c.call("getHealth", "/api/v1/health", "", 200, nil)
	c.call("getRiskPolicy", "/api/v1/risk/policy", "", 200, nil)
	c.call("getDashboard", "/api/v1/dashboard", "", 200, nil)
	c.call("loadFixtures", "/api/v1/fixtures", "{}", 200, nil)
	account := c.call("createPaperAccount", "/api/v1/paper/accounts", `{"currency":"USD","amount_minor":10000000}`, 200, nil)
	id := account["id"].(string)
	rec := c.call("evaluatePaperCandidate", "/api/v1/paper/recommendations", `{"account_id":"`+id+`","symbol":"AAPL","lane":"US_SWING","quantity":10,"entry_minor":20000,"stop_minor":19500}`, 200, nil)
	if rec["status"] != "PROPOSED" {
		t.Fatalf("candidate not proposed: %v", rec)
	}
	recID := rec["id"].(string)
	c.call("acknowledgePaperDecision", "/api/v1/paper/recommendations/"+recID+"/acknowledge", "{}", 200, nil)
	c.call("recordPaperFill", "/api/v1/paper/recommendations/"+recID+"/fill", `{"price_minor":20000,"confirmation":"PAPER_ONLY"}`, 200, nil)
	c.call("recordPaperSell", "/api/v1/paper/accounts/"+id+"/sell", `{"symbol":"AAPL","quantity":4,"price_minor":20100,"confirmation":"PAPER_ONLY"}`, 200, nil)
	c.call("evaluatePaperCandidate", "/api/v1/paper/recommendations", `{"account_id":"`+id+`","symbol":"MISSING","lane":"US_SWING","quantity":1,"entry_minor":10000,"stop_minor":9500}`, 200, nil)
	other := c.call("evaluatePaperCandidate", "/api/v1/paper/recommendations", `{"account_id":"`+id+`","symbol":"MSFT","lane":"US_SWING","quantity":2,"entry_minor":40000,"stop_minor":39000}`, 200, nil)
	c.call("cancelPaperDecision", "/api/v1/paper/recommendations/"+other["id"].(string)+"/cancel", "{}", 200, nil)
	c.call("setPaperEntries", "/api/v1/paper/entries", `{"paused":true}`, 200, nil)
	c.call("requestAssessmentStub", "/api/v1/research/assessments", `{"symbol":"AAPL"}`, 200, nil)
	full := c.call("getDashboard", "/api/v1/dashboard", "", 200, nil)
	if len(full["audit"].([]any)) < 8 || len(full["journal"].([]any)) < 4 {
		t.Fatalf("dashboard does not cover the audit and journal kinds: %v", full)
	}
	c.capture["getDashboard.200.populated"] = c.last
	// Legacy bound (043): the server does not check a symbol's length; a near-limit
	// unknown symbol is accepted, stored as a no-entry result and must still
	// match the documented answer and dashboard.
	long := strings.Repeat("Z", 65000)
	odd := c.call("evaluatePaperCandidate", "/api/v1/paper/recommendations", `{"account_id":"`+id+`","symbol":"`+long+`","lane":"US_SWING","quantity":1,"entry_minor":10000,"stop_minor":9500}`, 200, nil)
	if odd["status"] != "NO_TRADE" || odd["symbol"] != long {
		t.Fatalf("long unknown symbol: %v", odd["status"])
	}
	c.call("getDashboard", "/api/v1/dashboard", "", 200, nil)
	// Error families, each against its documented Error schema.
	c.token = "wrong-token-wrong-token-wrong"
	c.call("getDashboard", "/api/v1/dashboard", "", 401, nil)
	c.token = token
	c.call("getLive", "/api/v1/live", "", 403, map[string]string{"Origin": "https://evil.invalid"})
	c.call("createPaperAccount", "/api/v1/paper/accounts", `{"currency":"EUR","amount_minor":1}`, 422, nil)
	// The documented stop_minor minimum 1 is the existing rule (validMoney): 0 is refused.
	c.call("evaluatePaperCandidate", "/api/v1/paper/recommendations", `{"account_id":"`+id+`","symbol":"AAPL","lane":"US_SWING","quantity":1,"entry_minor":20000,"stop_minor":0}`, 422, nil)
	c.call("createPaperAccount", "/api/v1/paper/accounts", `{"currency":"USD","amount_minor":1}`, 415, map[string]string{"Content-Type": "text/plain"})
	c.call("createPaperAccount", "/api/v1/paper/accounts", strings.Repeat(" ", 65537), 413, nil)
	c.call("acknowledgePaperDecision", "/api/v1/paper/recommendations/"+strings.Repeat("0", 32)+"/acknowledge", "{}", 404, nil)
	c.call("recordPaperFill", "/api/v1/paper/recommendations/"+recID+"/fill", `{"price_minor":20000,"confirmation":"PAPER_ONLY"}`, 409, nil)

	if os.Getenv("UPDATE_CONTRACT_FIXTURES") == "1" {
		dir := "../../contracts/fixtures/api/v1"
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range c.capture {
			if err := os.WriteFile(filepath.Join(dir, name+".json"), append(bytes.TrimSpace(body), '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// The stored API golden fixtures still match the contract (Python checks the
// same files).
func TestAPIGoldenFixturesMatchTheContract(t *testing.T) {
	r := apiRegistry(t)
	ops := map[string]apiOperation{}
	for _, op := range operations(t, r) {
		ops[op.id] = op
	}
	files, err := filepath.Glob("../../contracts/fixtures/api/v1/*.json")
	if err != nil || len(files) < 15 {
		t.Fatalf("%d API fixtures: %v", len(files), err)
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		opID, rest, _ := strings.Cut(name, ".")
		status, _, _ := strings.Cut(rest, ".")
		code, err := strconv.Atoi(status)
		op, ok := ops[opID]
		if err != nil || !ok {
			t.Fatalf("fixture %s names no documented answer", name)
		}
		raw, _ := os.ReadFile(f)
		v, err := contracts.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if violation := r.Validate(responseSchema(t, op, code), v); violation != nil {
			t.Errorf("%s: %v", name, violation)
		}
	}
}

func TestAPIConformanceCorpus(t *testing.T) {
	r := apiRegistry(t)
	raw, err := os.ReadFile("../../contracts/fixtures/conformance/api.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := contracts.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range corpus.(map[string]any)["cases"].([]any) {
		c := item.(map[string]any)
		v, err := contracts.Parse([]byte(c["raw"].(string)))
		if err != nil {
			t.Fatalf("%s: %v", c["name"], err)
		}
		got := "valid"
		if violation := r.Validate(c["ref"].(string), v); violation != nil {
			got = violation.Keyword + "@" + violation.Path
		}
		want, ok := c["expect"].(string)
		if !ok {
			e := c["expect"].(map[string]any)
			want = e["keyword"].(string) + "@" + e["path"].(string)
		}
		if got != want {
			t.Errorf("%s: got %s, want %s", c["name"], got, want)
		}
	}
}
