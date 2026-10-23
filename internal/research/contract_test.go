package research

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"tradestocker/internal/contracts"
	"tradestocker/internal/telemetry"
)

// The Go/Python boundary that exists today: the SEC parser's request (Go to
// Python) and report (Python to Go), against contracts/research.
func researchContracts(t *testing.T) *contracts.Registry {
	t.Helper()
	r, err := contracts.NewRegistry(os.DirFS("../../contracts"),
		"research/sec-annual-v2.request.schema.json", "research/sec-annual-v2.report.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range r.Names() {
		if problems := r.CheckDialect(name, contracts.ServiceProfile); len(problems) > 0 {
			t.Fatalf("%s:\n  %s", name, strings.Join(problems, "\n  "))
		}
	}
	return r
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../contracts/fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestResearchContractCorpus(t *testing.T) {
	r := researchContracts(t)
	corpus, err := contracts.Parse(readFixture(t, "conformance/research.v1.json"))
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

// The request Go actually writes on the parser's stdin matches the contract.
func TestGoBuildsTheParserRequestTheContractDescribes(t *testing.T) {
	r := researchContracts(t)
	for _, asOf := range []time.Time{time.Now(), time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 23, 3, 0, 0, 120000, time.FixedZone("+03", 3*3600))} {
		body, _, _, err := parserRequest(fixtureJob(), fixtureSources(), asOf)
		if err != nil {
			t.Fatal(err)
		}
		v, err := contracts.Parse(body)
		if err != nil {
			t.Fatal(err)
		}
		if violation := r.Validate("research/sec-annual-v2.request.schema.json", v); violation != nil {
			t.Fatalf("Go's parser request at %v breaks its contract: %v", asOf, violation)
		}
	}
}

// Cross-language golden through the path Go really runs: the launcher with the
// embedded financials.py turns the golden request into exactly the golden
// report bytes, which pass the report contract and Go's own gate.
func TestTheActualParserPathProducesTheGoldenReport(t *testing.T) {
	requirePython(t)
	r := researchContracts(t)
	request := readFixture(t, "research/sec-annual-v2.request.json")
	golden := readFixture(t, "research/sec-annual-v2.report.json")
	ctx := telemetry.WithTrace(context.Background(), telemetry.NewTrace())
	cmd := (&Service{}).pythonCommand(ctx)
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(request), &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %s", err, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), golden) {
		t.Fatalf("parser output differs from the golden report (%d vs %d bytes)", stdout.Len(), len(golden))
	}
	report, err := contracts.Parse(golden)
	if err != nil {
		t.Fatal(err)
	}
	if violation := r.Validate("research/sec-annual-v2.report.schema.json", report); violation != nil {
		t.Fatalf("golden report breaks its contract: %v", violation)
	}
	req, _ := contracts.Parse(request)
	if violation := r.Validate("research/sec-annual-v2.request.schema.json", req); violation != nil {
		t.Fatalf("golden request breaks its contract: %v", violation)
	}
	fields := req.(map[string]any)
	if err := checkReport(bytes.TrimSuffix(golden, []byte("\n")), Job{Symbol: "TEST", CIK: "0000000001"}, fields["submissions_sha256"].(string), fields["facts_sha256"].(string)); err != nil {
		t.Fatalf("Go's report gate refuses the golden report: %v", err)
	}
}

// Both CIK encodings SEC uses (a padded string or an integer) in either
// original reach the parser unchanged, match the request contract and are
// accepted by the parser and Go's report gate (043).
func TestBothSECCIKEncodingsCrossTheBoundary(t *testing.T) {
	requirePython(t)
	r := researchContracts(t)
	subInt := strings.Replace(fixtureSubmissions, `"cik":"0000000001"`, `"cik":1`, 1)
	factsText := strings.Replace(fixtureFacts, `"cik":1`, `"cik":"0000000001"`, 1)
	factsShort := strings.Replace(fixtureFacts, `"cik":1`, `"cik":"1"`, 1)
	for name, pair := range map[string][2]string{
		"submissions integer": {subInt, fixtureFacts}, "companyfacts padded string": {fixtureSubmissions, factsText},
		"companyfacts short string": {fixtureSubmissions, factsShort}, "both swapped": {subInt, factsText},
	} {
		t.Run(name, func(t *testing.T) {
			sources := []Source{fixtureSource("submissions", pair[0]), fixtureSource("companyfacts", pair[1])}
			body, sub, facts, err := parserRequest(fixtureJob(), sources, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			// Go keeps each original's JSON value but re-encodes it as
			// json.Marshal does for a RawMessage: compact, with <, > and &
			// escaped. The SHA-256 fields still name the stored original bytes.
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatal(err)
			}
			for i, kind := range []string{"submissions", "companyfacts"} {
				want, _ := json.Marshal(json.RawMessage(pair[i]))
				if !bytes.Equal(fields[kind], want) {
					t.Fatalf("%s is not the original value re-encoded compactly", kind)
				}
			}
			v, _ := contracts.Parse(body)
			if violation := r.Validate("research/sec-annual-v2.request.schema.json", v); violation != nil {
				t.Fatalf("request contract refuses an encoding the parser accepts: %v", violation)
			}
			report, err := (&Service{}).analyze(context.Background(), fixtureJob(), sources)
			if err != nil {
				t.Fatalf("parser refused it: %v", err)
			}
			rv, _ := contracts.Parse(report)
			if violation := r.Validate("research/sec-annual-v2.report.schema.json", rv); violation != nil {
				t.Fatalf("report: %v", violation)
			}
			if err := checkReport(report, fixtureJob(), sub.SHA256, facts.SHA256); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The re-encoding keeps the value: an issuer name with & < > reaches the
// parser as the same string and the report names it unchanged.
func TestReencodedOriginalsKeepTheirValue(t *testing.T) {
	requirePython(t)
	sub := strings.Replace(fixtureSubmissions, `"name":"Synthetic Test Issuer"`, `"name":"Synthetic <Test> & Issuer"`, 1)
	sources := []Source{fixtureSource("submissions", sub), fixtureSource("companyfacts", fixtureFacts)}
	body, _, _, err := parserRequest(fixtureJob(), sources, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`Synthetic \u003cTest\u003e \u0026 Issuer`)) {
		t.Fatal("expected json.Marshal's escaping of < > &")
	}
	report, err := (&Service{}).analyze(context.Background(), fixtureJob(), sources)
	if err != nil {
		t.Fatal(err)
	}
	var r struct{ Issuer string }
	if err := json.Unmarshal(report, &r); err != nil || r.Issuer != "Synthetic <Test> & Issuer" {
		t.Fatalf("issuer %q %v", r.Issuer, err)
	}
}

// Research API golden payloads, built from the real Go types (Job, Source,
// Event) and wrapped as the handlers wrap them; the live handler answers are
// checked against the same contract by TestLiveResearchAnswersMatchTheContract
// on a disposable database. UPDATE_CONTRACT_FIXTURES=1 rewrites them.
func TestResearchAPIGoldenPayloads(t *testing.T) {
	r, err := contracts.NewRegistry(os.DirFS("../../contracts"), "openapi/local-api.v1.json",
		"research/sec-annual-v2.report.schema.json", "research/sec-annual-v2.request.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 23, 9, 0, 0, 123456000, time.UTC)
	next := at.Add(30 * time.Second)
	trace, code := "4bf92f3577b34da6a3ce929d0e0e4736", "SEC_HTTP_ERROR"
	report := bytes.TrimSuffix(readFixture(t, "research/sec-annual-v2.report.json"), []byte("\n"))
	done := Job{ID: "research_0123456789abcdef0123456789abcdef", Symbol: "TEST", CIK: "0000000001", Status: "SUCCEEDED", Attempts: 2,
		MaxAttempts: maxAttempts, CreatedAt: at, UpdatedAt: at.Add(time.Minute), Result: report, TraceID: &trace}
	queued := Job{ID: "research_fedcba9876543210fedcba9876543210", Symbol: "TEST", CIK: "0000000001", Status: "QUEUED", Attempts: 1,
		MaxAttempts: maxAttempts, CreatedAt: at, UpdatedAt: at, NextAttemptAt: &next, LastError: &code}
	listed := done
	listed.Result = nil
	sources := []Source{}
	for _, s := range fixtureSources() {
		s.Attempt, s.ReceivedAt = 2, at
		sources = append(sources, s)
	}
	event := func(action, detail string) Event {
		return Event{At: at, Action: action, Detail: json.RawMessage(detail)}
	}
	events := []Event{
		event("REQUESTED", `{"trace_id":"`+trace+`"}`), event("ATTEMPT_STARTED", `{"attempt":1}`),
		event("ATTEMPT_FAILED_RETRY_SCHEDULED", `{"attempt":1,"code":"SEC_HTTP_ERROR","retry_after_seconds":30}`),
		event("LEASE_RECOVERED", `{"previous_attempt":1}`), event("ATTEMPT_STARTED", `{"attempt":2}`),
		event("SEC_SOURCES_STORED", `{"attempt":2,"sources":[{"kind":"submissions","sha256":"`+sources[0].SHA256+`","size_bytes":10}]}`),
		event("PROVIDER_COOLDOWN_STARTED", `{"provider":"SEC","code":"SEC_ACCESS_LIMITED","until":"2026-10-23T09:10:00Z"}`),
		event("SUCCEEDED", `{"attempt":2,"parser_version":"sec-annual-v2"}`),
	}
	cooldown := at.Add(10 * time.Minute)
	payloads := map[string]struct {
		ref   string
		value any
	}{
		"createResearchJob.202": {"Job", queued},
		"listResearchJobs.200":  {"JobList", map[string]any{"jobs": []Job{listed, queued}}},
		"getResearchJob.200":    {"JobDetail", map[string]any{"job": done, "sources": sources, "events": events}},
		"getResearchStatus.200": {"ResearchStatus", map[string]any{"configured": true, "source": "SEC_PUBLIC", "mode": "READ_ONLY_RESEARCH", "parser_version": ParserVersion, "max_attempts": maxAttempts, "provider_cooldown_until": &cooldown, "live_advisory_enabled": false}},
		"getResearchJob.404":    {"Error", map[string]any{"error": map[string]string{"code": "RESEARCH_NOT_FOUND", "message": messages["RESEARCH_NOT_FOUND"]}}},
		"createResearchJob.409": {"Error", map[string]any{"error": map[string]string{"code": "IDEMPOTENCY_CONFLICT", "message": messages["IDEMPOTENCY_CONFLICT"]}}},
		"getResearchSource.200": {"SecOriginal", json.RawMessage(fixtureFacts)},
		"getResearchSource.422": {"Error", map[string]any{"error": map[string]string{"code": "INVALID_SOURCE_HASH", "message": messages["INVALID_SOURCE_HASH"]}}},
	}
	for name, p := range payloads {
		raw, err := json.Marshal(p.value)
		if err != nil {
			t.Fatal(err)
		}
		v, err := contracts.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if violation := r.Validate("openapi/local-api.v1.json#/components/schemas/"+p.ref, v); violation != nil {
			t.Errorf("%s breaks its contract: %v", name, violation)
		}
		if os.Getenv("UPDATE_CONTRACT_FIXTURES") == "1" {
			if err := os.WriteFile("../../contracts/fixtures/api/v1/"+name+".json", append(raw, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// The live research handler's answers, on a disposable database, match the
// documented responses for each status: create, replay, conflict, the request
// errors, list, detail (after a retry and an SEC access limit), sources and
// status, including every timestamp read back from PostgreSQL.
func TestLiveResearchAnswersMatchTheContract(t *testing.T) {
	requirePython(t)
	f := &scriptedFetcher{results: []error{retryable("SEC_HTTP_ERROR"), retryable("SEC_ACCESS_LIMITED")}}
	s := integrationService(t, f)
	api, err := contracts.NewRegistry(os.DirFS("../../contracts"), "openapi/local-api.v1.json",
		"research/sec-annual-v2.report.schema.json", "research/sec-annual-v2.request.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	ops := map[string]string{"POST /api/v1/research/jobs": "/api/v1/research/jobs", "GET /api/v1/research/jobs": "/api/v1/research/jobs",
		"GET /api/v1/research/jobs/{id}": "/api/v1/research/jobs/{id}", "GET /api/v1/research/jobs/{id}/sources/{kind}": "/api/v1/research/jobs/{id}/sources/{kind}",
		"GET /api/v1/research/status": "/api/v1/research/status"}
	h := s.Handler()
	check := func(op, target, body string, headers map[string]string, want int) []byte {
		t.Helper()
		method, _, _ := strings.Cut(op, " ")
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, target, w.Code, want, w.Body)
		}
		v, err := contracts.Parse(w.Body.Bytes())
		if err != nil {
			t.Fatalf("%s: answer is not strict JSON: %v", target, err)
		}
		ref := fmt.Sprintf("openapi/local-api.v1.json#/paths/%s/%s/responses/%d/content/application~1json/schema",
			strings.ReplaceAll(ops[op], "/", "~1"), strings.ToLower(method), w.Code)
		if violation := api.Validate(ref, v); violation != nil {
			t.Fatalf("%s %s %d breaks its contract: %v\n%s", method, target, w.Code, violation, w.Body)
		}
		return w.Body.Bytes()
	}
	jsonHeaders := func(key string) map[string]string {
		return map[string]string{"Content-Type": "application/json", "Idempotency-Key": key}
	}
	check("GET /api/v1/research/status", "/api/v1/research/status", "", nil, 200)
	body := check("POST /api/v1/research/jobs", "/api/v1/research/jobs", fixtureRequest, jsonHeaders("contract-live-1"), 202)
	var job Job
	if err := json.Unmarshal(body, &job); err != nil {
		t.Fatal(err)
	}
	check("POST /api/v1/research/jobs", "/api/v1/research/jobs", fixtureRequest, jsonHeaders("contract-live-1"), 202)
	check("POST /api/v1/research/jobs", "/api/v1/research/jobs", `{"symbol":"AAPL","cik":"0000320193"}`, jsonHeaders("contract-live-1"), 409)
	check("POST /api/v1/research/jobs", "/api/v1/research/jobs", fixtureRequest, map[string]string{"Idempotency-Key": "contract-live-2"}, 415)
	check("POST /api/v1/research/jobs", "/api/v1/research/jobs", `{"symbol":"test"}`, jsonHeaders("contract-live-3"), 422)
	check("POST /api/v1/research/jobs", "/api/v1/research/jobs", strings.Repeat(" ", 5000), jsonHeaders("contract-live-4"), 413)
	check("GET /api/v1/research/jobs", "/api/v1/research/jobs", "", nil, 200)
	// Attempt 1 fails and is retried; attempt 2 meets an SEC access limit
	// (cooldown event with its until time); attempt 3 succeeds.
	for i := 0; i < 3; i++ {
		if _, err := s.Pool.Exec(context.Background(), `UPDATE research_jobs SET not_before=now()-interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Pool.Exec(context.Background(), `UPDATE research_provider_state SET not_before=now()-interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		processOnce(t, s)
		check("GET /api/v1/research/jobs/{id}", "/api/v1/research/jobs/"+job.ID, "", nil, 200)
		check("GET /api/v1/research/status", "/api/v1/research/status", "", nil, 200)
	}
	detail := check("GET /api/v1/research/jobs/{id}", "/api/v1/research/jobs/"+job.ID, "", nil, 200)
	if !bytes.Contains(detail, []byte(`"SUCCEEDED"`)) || !bytes.Contains(detail, []byte("PROVIDER_COOLDOWN_STARTED")) {
		t.Fatalf("job did not go through the retry, cooldown and success: %s", detail)
	}
	check("GET /api/v1/research/jobs", "/api/v1/research/jobs", "", nil, 200)
	sha := fixtureSources()[1].SHA256
	check("GET /api/v1/research/jobs/{id}/sources/{kind}", "/api/v1/research/jobs/"+job.ID+"/sources/companyfacts", "", nil, 200)
	check("GET /api/v1/research/jobs/{id}/sources/{kind}", "/api/v1/research/jobs/"+job.ID+"/sources/companyfacts?sha256="+sha, "", nil, 200)
	check("GET /api/v1/research/jobs/{id}/sources/{kind}", "/api/v1/research/jobs/"+job.ID+"/sources/companyfacts?sha256=XYZ", "", nil, 422)
	check("GET /api/v1/research/jobs/{id}/sources/{kind}", "/api/v1/research/jobs/"+job.ID+"/sources/other", "", nil, 404)
	check("GET /api/v1/research/jobs/{id}", "/api/v1/research/jobs/research_00000000000000000000000000000000", "", nil, 404)
}
