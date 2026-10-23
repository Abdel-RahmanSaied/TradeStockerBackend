package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Synthetic SEC-shaped issuer used by service tests. It is test data, not a real filing.
const fixtureSymbol, fixtureCIK = "TEST", "0000000001"

const fixtureSubmissions = `{"cik":"0000000001","name":"Synthetic Test Issuer","tickers":["TEST"],"sic":"3571","filings":{"recent":{
"accessionNumber":["0000000001-25-000020"],"form":["10-K"],"filingDate":["2025-10-31"],"reportDate":["2025-09-27"],"acceptanceDateTime":["2025-10-31T14:01:26.000Z"]}}}`

const fixtureFacts = `{"cik":1,"entityName":"Synthetic Test Issuer","facts":{"us-gaap":{
"RevenueFromContractWithCustomerExcludingAssessedTax":{"units":{"USD":[{"start":"2024-09-29","end":"2025-09-27","val":1200,"accn":"0000000001-25-000020","form":"10-K","filed":"2025-10-31"}]}},
"NetIncomeLoss":{"units":{"USD":[{"start":"2024-09-29","end":"2025-09-27","val":120,"accn":"0000000001-25-000020","form":"10-K","filed":"2025-10-31"}]}},
"Assets":{"units":{"USD":[{"end":"2025-09-27","val":5500,"accn":"0000000001-25-000020","form":"10-K","filed":"2025-10-31"}]}}}}}`

func fixtureSource(kind, body string) Source {
	sum := sha256.Sum256([]byte(body))
	u := "https://data.sec.gov/submissions/CIK" + fixtureCIK + ".json"
	if kind == "companyfacts" {
		u = "https://data.sec.gov/api/xbrl/companyfacts/CIK" + fixtureCIK + ".json"
	}
	return Source{Kind: kind, URL: u, Body: []byte(body), Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), ReceivedAt: time.Now().UTC()}
}

func fixtureSources() []Source {
	return []Source{fixtureSource("submissions", fixtureSubmissions), fixtureSource("companyfacts", fixtureFacts)}
}

// scriptedFetcher returns queued results in order, then fixture sources.
type scriptedFetcher struct {
	mu      sync.Mutex
	results []error
	calls   int
	block   chan struct{} // when set, Fetch waits for it or for cancellation
	onFetch func()        // when set, runs just before Fetch returns
}

func (f *scriptedFetcher) Fetch(ctx context.Context, symbol, cik string) ([]Source, error) {
	f.mu.Lock()
	f.calls++
	var next error
	if len(f.results) > 0 {
		next, f.results = f.results[0], f.results[1:]
	}
	block, onFetch := f.block, f.onFetch
	f.mu.Unlock()
	if onFetch != nil {
		defer onFetch()
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if next != nil {
		return nil, next
	}
	return fixtureSources(), nil
}

func (f *scriptedFetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
