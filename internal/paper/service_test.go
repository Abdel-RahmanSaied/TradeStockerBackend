package paper

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

func setup(t *testing.T) (State, Account) {
	t.Helper()
	s := NewState()
	a, e := s.CreateAccount("USD", 1_000_000, now)
	if e != nil {
		t.Fatal(e)
	}
	s.LoadFixtures(now)
	return s, a
}
func candidate(a Account) Candidate { return Candidate{a.ID, "AAPL", "US_SWING", 1, 20000, 19500} }
func check(t *testing.T, s *State) {
	t.Helper()
	if e := s.Validate(); e != nil {
		t.Fatal(e)
	}
}
func TestHumanPaperLifecycle(t *testing.T) {
	s, a := setup(t)
	r, e := s.Evaluate(candidate(a), now)
	if e != nil || r.Status != "PROPOSED" {
		t.Fatalf("%+v %v", r, e)
	}
	if s.Accounts[0].Cash != 1_000_000 || s.Accounts[0].Reserved != 20000 {
		t.Fatal("reservation changed cash")
	}
	if _, e = s.Fill(r.ID, 20000, "PAPER_ONLY", now); e == nil {
		t.Fatal("filled before acknowledgment")
	}
	if _, e = s.Acknowledge(r.ID, now); e != nil {
		t.Fatal(e)
	}
	if len(s.Positions) != 0 || s.Accounts[0].Cash != 1_000_000 {
		t.Fatal("acknowledgment is not execution")
	}
	if _, e = s.Fill(r.ID, 19900, "PAPER_ONLY", now); e != nil {
		t.Fatal(e)
	}
	if s.Accounts[0].Cash != 980100 || s.Accounts[0].Reserved != 0 || s.Positions[0].Quantity != 1 {
		t.Fatal("incorrect financial effect")
	}
	if _, e = s.Fill(r.ID, 19900, "PAPER_ONLY", now); e == nil {
		t.Fatal("duplicate fill permitted")
	}
	check(t, &s)
}
func TestUnsafeCandidatesFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Candidate, *State)
		reason string
	}{
		{"missing evidence", func(c *Candidate, s *State) { c.Symbol = "MISSING"; c.Entry = 10000; c.Stop = 9000 }, "FINANCIAL_OR_HISTORY"},
		{"stale", func(c *Candidate, s *State) { c.Symbol = "STALE"; c.Entry = 10000; c.Stop = 9000 }, "STALE_MISSING"},
		{"future", func(c *Candidate, s *State) { s.Quotes[0].AsOf = now.Add(time.Minute) }, "FUTURE_DATA"},
		{"wrong market", func(c *Candidate, s *State) { c.Lane = "EGX_SWING" }, "MARKET_MISMATCH"},
		{"wrong currency", func(c *Candidate, s *State) { s.Accounts[0].Currency = "EGP" }, "CURRENCY_OR_MARKET"},
		{"actual input", func(c *Candidate, s *State) { s.Quotes[0].Mode = "ACTUAL" }, "MODE_CURRENCY"},
		{"intraday eligibility", func(c *Candidate, s *State) { c.Lane = "US_INTRADAY" }, "INTRADAY_ELIGIBILITY"},
		{"paused", func(c *Candidate, s *State) { s.EntriesPaused = true }, "ENTRIES_PAUSED"},
		{"too much risk", func(c *Candidate, s *State) { c.Stop = 1 }, "PER_TRADE_RISK"},
		{"too much exposure", func(c *Candidate, s *State) { c.Quantity = 11 }, "SYMBOL_EXPOSURE"},
		{"too much cash", func(c *Candidate, s *State) { c.Quantity = 51 }, "INSUFFICIENT_AVAILABLE"},
		{"invented entry", func(c *Candidate, s *State) { c.Entry = 100; c.Stop = 99 }, "MATCH_SYNTHETIC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, a := setup(t)
			c := candidate(a)
			tc.change(&c, &s)
			r, e := s.Evaluate(c, now)
			if e != nil {
				t.Fatal(e)
			}
			if r.Status == "PROPOSED" || !strings.Contains(strings.Join(r.Reasons, " "), tc.reason) {
				t.Fatalf("unsafe result %+v", r)
			}
			if s.Accounts[0].Reserved != 0 {
				t.Fatal("rejected candidate reserved money")
			}
			check(t, &s)
		})
	}
}
func TestMalformedFinancialInputs(t *testing.T) {
	for _, mutate := range []func(*Candidate){func(c *Candidate) { c.Quantity = -1 }, func(c *Candidate) { c.Quantity = MaxQuantity + 1 }, func(c *Candidate) { c.Entry = MaxMoney + 1 }, func(c *Candidate) { c.Stop = c.Entry }, func(c *Candidate) { c.Lane = "LIVE" }} {
		s, a := setup(t)
		c := candidate(a)
		mutate(&c)
		if _, e := s.Evaluate(c, now); e == nil {
			t.Fatal("invalid candidate accepted")
		}
		check(t, &s)
	}
}
func TestReservationExposureIsShared(t *testing.T) {
	s, a := setup(t)
	c := candidate(a)
	c.Quantity = 6
	r, e := s.Evaluate(c, now)
	if e != nil || r.Status != "PROPOSED" {
		t.Fatal(r, e)
	}
	r, e = s.Evaluate(c, now)
	if e != nil || r.Status != "NO_TRADE" || s.Accounts[0].Reserved != 120000 {
		t.Fatal("double exposure budget", r, e)
	}
	check(t, &s)
}
func TestTotalOpenRiskAcrossSymbols(t *testing.T) {
	s, a := setup(t)
	s.Quotes = append(s.Quotes, Quote{"TEST", "US", 20000, "USD", now, "fixture", "SYNTHETIC", "fixture", true, true})
	for i, symbol := range []string{"AAPL", "MSFT", "TEST", "AAPL"} {
		c := candidate(a)
		c.Symbol = symbol
		if symbol == "MSFT" {
			c.Entry = 40000
			c.Stop = 30000
		} else {
			c.Stop = 10000
		}
		r, e := s.Evaluate(c, now)
		if e != nil {
			t.Fatal(e)
		}
		if i < 3 && r.Status != "PROPOSED" {
			t.Fatal(r)
		}
		if i == 3 && !strings.Contains(strings.Join(r.Reasons, " "), "TOTAL_OPEN_RISK") {
			t.Fatal(r)
		}
	}
	check(t, &s)
}
func TestExpiryAndCancelReleaseOnlyReservation(t *testing.T) {
	for _, expire := range []bool{false, true} {
		s, a := setup(t)
		r, e := s.Evaluate(candidate(a), now)
		if e != nil {
			t.Fatal(e)
		}
		if expire {
			s.Expire(now.Add(16 * time.Minute))
		} else {
			_, e = s.Cancel(r.ID, now)
			if e != nil {
				t.Fatal(e)
			}
		}
		if s.Accounts[0].Cash != 1_000_000 || s.Accounts[0].Reserved != 0 || len(s.Positions) != 0 {
			t.Fatal("release affected cash/positions")
		}
		if _, e = s.Acknowledge(r.ID, now); e == nil {
			t.Fatal("terminal decision acknowledged")
		}
		check(t, &s)
	}
}
func TestPaperFillGates(t *testing.T) {
	for _, tc := range []struct {
		price   int64
		confirm string
		at      time.Time
	}{{20001, "PAPER_ONLY", now}, {19500, "PAPER_ONLY", now}, {20000, "ACTUAL", now}, {20000, "PAPER_ONLY", now.Add(6 * time.Minute)}} {
		s, a := setup(t)
		r, _ := s.Evaluate(candidate(a), now)
		s.Acknowledge(r.ID, now)
		if _, e := s.Fill(r.ID, tc.price, tc.confirm, tc.at); e == nil {
			t.Fatal("unsafe paper fill accepted")
		}
		if len(s.Positions) != 0 {
			t.Fatal("failed fill created position")
		}
		check(t, &s)
	}
}
func TestPaperSellCannotShort(t *testing.T) {
	s, a := setup(t)
	r, _ := s.Evaluate(candidate(a), now)
	s.Acknowledge(r.ID, now)
	s.Fill(r.ID, 20000, "PAPER_ONLY", now)
	if _, e := s.Sell(a.ID, "AAPL", 2, 20000, "PAPER_ONLY", now); e == nil {
		t.Fatal("oversold")
	}
	p, e := s.Sell(a.ID, "AAPL", 1, 19000, "PAPER_ONLY", now)
	if e != nil || p.Quantity != 0 || s.Accounts[0].Cash != 999000 {
		t.Fatal(p, e)
	}
	check(t, &s)
}
func TestSyntheticLabelsAndDryRun(t *testing.T) {
	s, a := setup(t)
	s.Evaluate(candidate(a), now)
	for _, q := range s.Quotes {
		if q.Mode != "SYNTHETIC" || q.Source != "bundled-fixture-v1" {
			t.Fatal("unlabeled fixture")
		}
	}
	for _, n := range s.Notifications {
		if n.Status != "DRY_RUN_NOT_SENT" || !strings.HasPrefix(n.Text, "[PAPER / SYNTHETIC]") {
			t.Fatal("unsafe notification")
		}
	}
	if s.Live {
		t.Fatal("live enabled")
	}
	check(t, &s)
}
func TestCorruptFinancialStateRefused(t *testing.T) {
	s, a := setup(t)
	s.Accounts[0].Cash++
	if s.Validate() == nil {
		t.Fatal("journal drift accepted")
	}
	s, _ = setup(t)
	s.Accounts[0].Reserved = 1
	if s.Validate() == nil {
		t.Fatal("reservation drift accepted")
	}
	s, _ = setup(t)
	s.Live = true
	if s.Validate() == nil {
		t.Fatal("live accepted")
	}
	_ = a
}
