package paper

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Fault struct {
	Code    string
	Message string
	Status  int
}

func (f *Fault) Error() string                { return f.Message }
func Fail(code, msg string, status int) error { return &Fault{code, msg, status} }
func (s *State) Account(id string) (*Account, error) {
	for i := range s.Accounts {
		if s.Accounts[i].ID == id {
			return &s.Accounts[i], nil
		}
	}
	return nil, Fail("NOT_FOUND", "Paper account not found", 404)
}
func (s *State) Recommendation(id string) (*Recommendation, error) {
	for i := range s.Recommendations {
		if s.Recommendations[i].ID == id {
			return &s.Recommendations[i], nil
		}
	}
	return nil, Fail("NOT_FOUND", "Paper decision not found", 404)
}
func validMoney(v int64) bool { return v > 0 && v <= MaxMoney }
func (s *State) CreateAccount(currency string, amount int64, now time.Time) (Account, error) {
	if currency != "USD" && currency != "EGP" {
		return Account{}, Fail("VALIDATION_FAILED", "Currency must be USD or EGP", 422)
	}
	if !validMoney(amount) {
		return Account{}, Fail("VALIDATION_FAILED", "Synthetic starting balance is out of bounds", 422)
	}
	for _, a := range s.Accounts {
		if a.Currency == currency {
			return Account{}, Fail("ACCOUNT_EXISTS", "A paper account already exists for this currency", 409)
		}
	}
	a := Account{ID: ID(), Currency: currency, Mode: "PAPER"}
	s.Line(now, &a, a.ID, "PAPER_DEPOSIT", amount, 0)
	s.Accounts = append(s.Accounts, a)
	s.Event(now, "PAPER_ACCOUNT_CREATED", a.ID, map[string]any{"currency": currency, "synthetic_funds_minor": amount})
	return a, nil
}
func (s *State) LoadFixtures(now time.Time) map[string]any {
	// These values are invented test inputs, never fetched or presented as real prices.
	s.Quotes = []Quote{
		{"AAPL", "US", 20000, "USD", now, "bundled-fixture-v1", "SYNTHETIC", "fixture-financial-history-us-v1", true, true},
		{"MSFT", "US", 40000, "USD", now, "bundled-fixture-v1", "SYNTHETIC", "fixture-financial-history-us-v1", true, true},
		{"COMI", "EGX", 8000, "EGP", now, "bundled-fixture-v1", "SYNTHETIC", "fixture-financial-history-egx-v1", true, true},
		{"MISSING", "US", 10000, "USD", now, "bundled-fixture-v1", "SYNTHETIC", "", false, false},
		{"STALE", "US", 10000, "USD", now.Add(-24 * time.Hour), "bundled-fixture-v1", "SYNTHETIC", "fixture-stale-v1", true, true},
	}
	s.Event(now, "SYNTHETIC_FIXTURES_LOADED", "fixture-v1", map[string]any{"count": len(s.Quotes), "generated_at": now, "note": "Invented paper test prices and financial/history evidence; not real market information"})
	return map[string]any{"mode": "PAPER", "data_label": "SYNTHETIC", "count": len(s.Quotes)}
}

type Candidate struct {
	AccountID string `json:"account_id"`
	Symbol    string `json:"symbol"`
	Lane      string `json:"lane"`
	Quantity  int64  `json:"quantity"`
	Entry     int64  `json:"entry_minor"`
	Stop      int64  `json:"stop_minor"`
}

func active(status string) bool { return status == "PROPOSED" || status == "ACKNOWLEDGED" }

// ExpiryDue reports whether Expire(now) has work to do. It changes nothing, so a
// read can decide whether it must take the locked write path.
func (s *State) ExpiryDue(now time.Time) bool {
	for _, r := range s.Recommendations {
		if active(r.Status) && !now.Before(r.ExpiresAt) {
			return true
		}
	}
	return false
}
func (s *State) Expire(now time.Time) {
	for i := range s.Recommendations {
		r := &s.Recommendations[i]
		if active(r.Status) && !now.Before(r.ExpiresAt) {
			a, err := s.Account(r.AccountID)
			if err != nil {
				continue
			}
			s.Line(now, a, r.ID, "EXPIRE_RESERVATION", 0, -r.Reserved)
			r.Reserved = 0
			r.Status = "EXPIRED"
			s.Event(now, "PAPER_DECISION_EXPIRED", r.ID, "Unused paper reservation released; no position closed")
			s.Notify(now, "Paper decision expired: "+r.Symbol)
		}
	}
}
func (s *State) quote(symbol string) *Quote {
	for i := range s.Quotes {
		if s.Quotes[i].Symbol == symbol {
			return &s.Quotes[i]
		}
	}
	return nil
}
func Fresh(q *Quote, now time.Time) bool {
	return q != nil && !q.AsOf.IsZero() && now.Sub(q.AsOf) >= -5*time.Second && now.Sub(q.AsOf) <= Freshness
}

// projectedOpenRisk reflects the merged-position stop model, including every
// outstanding reservation. Separate cheap proposals cannot hide stop widening.
func (s *State) projectedOpenRisk(accountID string, extra *Candidate) int64 {
	type exposure struct{ quantity, cost, stop int64 }
	bySymbol := map[string]exposure{}
	add := func(symbol string, quantity, cost, stop int64) {
		if quantity <= 0 {
			return
		}
		x := bySymbol[symbol]
		if x.quantity == 0 {
			x.stop = stop
		} else {
			x.stop = min(x.stop, stop)
		}
		x.quantity += quantity
		x.cost += cost
		bySymbol[symbol] = x
	}
	for _, p := range s.Positions {
		if p.AccountID == accountID {
			add(p.Symbol, p.Quantity, p.Cost, p.Stop)
		}
	}
	for _, r := range s.Recommendations {
		if r.AccountID == accountID && active(r.Status) {
			add(r.Symbol, r.Quantity, r.Reserved, r.Stop)
		}
	}
	if extra != nil {
		add(extra.Symbol, extra.Quantity, extra.Quantity*extra.Entry, extra.Stop)
	}
	var risk int64
	for _, x := range bySymbol {
		risk += max(int64(0), x.cost-x.quantity*x.stop)
	}
	return risk
}

func (s *State) Evaluate(c Candidate, now time.Time) (Recommendation, error) {
	a, err := s.Account(c.AccountID)
	if err != nil {
		return Recommendation{}, err
	}
	if c.Quantity <= 0 || c.Quantity > MaxQuantity || !validMoney(c.Entry) || !validMoney(c.Stop) || c.Stop >= c.Entry {
		return Recommendation{}, Fail("VALIDATION_FAILED", "Use whole positive shares, positive entry and stop, and stop below entry", 422)
	}
	validLane := false
	for _, l := range s.Lanes {
		if c.Lane == l.ID {
			validLane = true
		}
	}
	if !validLane {
		return Recommendation{}, Fail("VALIDATION_FAILED", "Unknown lane", 422)
	}
	s.Expire(now)
	r := Recommendation{ID: ID(), Symbol: c.Symbol, Lane: c.Lane, AccountID: c.AccountID, Status: "PROPOSED", Quantity: c.Quantity, Entry: c.Entry, Stop: c.Stop, Currency: a.Currency, CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute), Reasons: []string{}, Mode: "PAPER", Versions: map[string]string{"policy": PolicyVersion, "strategy": "synthetic-candidate-v1", "model": "none-research-stub", "prompt": "none", "build": Build, "data": "bundled-fixture-v1"}}
	q := s.quote(c.Symbol)
	if q == nil || !q.FinancialValid || !q.HistoryValid || q.Assessment == "" {
		r.Status = "INSUFFICIENT_DATA"
		r.Reasons = append(r.Reasons, "FINANCIAL_OR_HISTORY_EVIDENCE_MISSING")
	}
	if !Fresh(q, now) {
		r.Status = "NO_TRADE"
		r.Reasons = append(r.Reasons, "STALE_MISSING_OR_FUTURE_DATA")
	}
	if q != nil {
		r.Versions["assessment"] = q.Assessment
		r.Versions["observation"] = q.AsOf.Format(time.RFC3339Nano)
		if q.Mode != "SYNTHETIC" || q.Currency != a.Currency || !strings.HasPrefix(c.Lane, q.Market+"_") {
			r.Status = "NO_TRADE"
			r.Reasons = append(r.Reasons, "MODE_CURRENCY_OR_MARKET_MISMATCH")
		}
		if c.Entry != q.Price {
			r.Status = "NO_TRADE"
			r.Reasons = append(r.Reasons, "ENTRY_MUST_MATCH_SYNTHETIC_REFERENCE")
		}
	}
	if strings.HasSuffix(c.Lane, "INTRADAY") {
		r.Status = "NO_TRADE"
		r.Reasons = append(r.Reasons, "INTRADAY_ELIGIBILITY_NOT_VALIDATED")
	}
	if s.EntriesPaused {
		r.Status = "NO_TRADE"
		r.Reasons = append(r.Reasons, "ENTRIES_PAUSED")
	}
	notional := c.Quantity * c.Entry
	risk := c.Quantity * (c.Entry - c.Stop)
	equity := a.Cash
	exposure := notional
	openRisk := s.projectedOpenRisk(a.ID, &c)
	for _, p := range s.Positions {
		if p.AccountID == a.ID {
			equity += p.Cost
			if p.Symbol == c.Symbol {
				exposure += p.Cost
			}
		}
	}
	for _, other := range s.Recommendations {
		if other.AccountID == a.ID && active(other.Status) {
			if other.Symbol == c.Symbol {
				exposure += other.Reserved
			}
		}
	}
	if notional > a.Cash-a.Reserved {
		r.Status = "NO_TRADE"
		r.Reasons = append(r.Reasons, "INSUFFICIENT_AVAILABLE_CASH")
	}
	if risk > equity/100 {
		r.Status = "NO_TRADE"
		r.Reasons = append(r.Reasons, "PAPER_PER_TRADE_RISK_EXCEEDED")
	}
	if openRisk > equity*3/100 {
		r.Status = "NO_TRADE"
		r.Reasons = append(r.Reasons, "PAPER_TOTAL_OPEN_RISK_EXCEEDED")
	}
	if exposure > equity/5 {
		r.Status = "NO_TRADE"
		r.Reasons = append(r.Reasons, "PAPER_SYMBOL_EXPOSURE_EXCEEDED")
	}
	if r.Status == "PROPOSED" {
		r.Reserved = notional
		r.Reasons = append(r.Reasons, "PAPER_TEST_CHECKS_PASSED_NOT_LIVE_ADVICE")
		s.Line(now, a, r.ID, "PAPER_ENTRY_RESERVATION", 0, notional)
	}
	s.Recommendations = append(s.Recommendations, r)
	s.Event(now, "PAPER_CANDIDATE_EVALUATED", r.ID, map[string]any{"status": r.Status, "reasons": r.Reasons, "versions": r.Versions})
	s.Notify(now, fmt.Sprintf("%s: %s (%s). Human-only paper workflow.", r.Symbol, r.Status, r.ID))
	return r, nil
}
func (s *State) Acknowledge(id string, now time.Time) (Recommendation, error) {
	r, err := s.Recommendation(id)
	if err != nil {
		return Recommendation{}, err
	}
	if r.Status != "PROPOSED" {
		return Recommendation{}, Fail("INVALID_STATE", "Only an unexpired proposed paper decision may be acknowledged", 409)
	}
	r.Status = "ACKNOWLEDGED"
	s.Event(now, "PAPER_ACKNOWLEDGED", id, "Acknowledged does not mean filled; no cash or position changed")
	return *r, nil
}
func (s *State) Cancel(id string, now time.Time) (Recommendation, error) {
	r, err := s.Recommendation(id)
	if err != nil {
		return Recommendation{}, err
	}
	if !active(r.Status) {
		return Recommendation{}, Fail("INVALID_STATE", "Only an open paper decision may be cancelled", 409)
	}
	a, err := s.Account(r.AccountID)
	if err != nil {
		return Recommendation{}, err
	}
	s.Line(now, a, r.ID, "CANCEL_RESERVATION", 0, -r.Reserved)
	r.Reserved = 0
	r.Status = "CANCELLED"
	s.Event(now, "PAPER_CANCELLED", id, "Reserved paper cash released")
	return *r, nil
}
func (s *State) Fill(id string, price int64, confirmation string, now time.Time) (Recommendation, error) {
	r, err := s.Recommendation(id)
	if err != nil {
		return Recommendation{}, err
	}
	if r.Status != "ACKNOWLEDGED" {
		return Recommendation{}, Fail("INVALID_STATE", "A human must acknowledge the paper decision before recording its fill", 409)
	}
	if confirmation != "PAPER_ONLY" || !validMoney(price) || price > r.Entry || price <= r.Stop {
		return Recommendation{}, Fail("VALIDATION_FAILED", "Confirm PAPER_ONLY and use a positive fill above stop and no higher than reserved entry", 422)
	}
	if !Fresh(s.quote(r.Symbol), now) {
		return Recommendation{}, Fail("STALE_DATA", "Fresh synthetic observation required to record this paper fill", 409)
	}
	a, err := s.Account(r.AccountID)
	if err != nil {
		return Recommendation{}, err
	}
	equity := a.Cash
	var exposure int64
	for _, p := range s.Positions {
		if p.AccountID == a.ID {
			equity += p.Cost
			if p.Symbol == r.Symbol {
				exposure += p.Cost
			}
		}
	}
	for _, pending := range s.Recommendations {
		if pending.AccountID == a.ID && pending.Symbol == r.Symbol && active(pending.Status) {
			exposure += pending.Reserved
		}
	}
	if s.projectedOpenRisk(a.ID, nil) > equity*3/100 || r.Quantity*(r.Entry-r.Stop) > equity/100 || exposure > equity/5 {
		return Recommendation{}, Fail("PAPER_RISK_RECHECK_FAILED", "Current merged-position risk or exposure exceeds the paper test budget", 409)
	}
	cost := price * r.Quantity
	if cost > r.Reserved || cost > a.Cash {
		return Recommendation{}, Fail("INSUFFICIENT_CASH", "Paper fill exceeds reserved cash", 409)
	}
	s.Line(now, a, r.ID, "HUMAN_PAPER_BUY", -cost, -r.Reserved)
	r.Reserved = 0
	r.Status = "FILLED"
	found := false
	for i := range s.Positions {
		p := &s.Positions[i]
		if p.AccountID == a.ID && p.Symbol == r.Symbol {
			if p.Quantity == 0 {
				p.Stop = r.Stop
			} else {
				p.Stop = min(p.Stop, r.Stop)
			}
			p.Quantity += r.Quantity
			p.Cost += cost
			found = true
		}
	}
	if !found {
		s.Positions = append(s.Positions, Position{Symbol: r.Symbol, Quantity: r.Quantity, Cost: cost, Stop: r.Stop, AccountID: a.ID, Currency: a.Currency})
	}
	s.Event(now, "HUMAN_PAPER_FILL_RECORDED", id, map[string]any{"price_minor": price, "quantity": r.Quantity, "fees": "zero-fee test assumption; not execution-quality evidence"})
	s.Notify(now, "Human paper fill recorded: "+r.Symbol+". No real order sent.")
	return *r, nil
}
func (s *State) Sell(accountID, symbol string, quantity, price int64, confirmation string, now time.Time) (Position, error) {
	if confirmation != "PAPER_ONLY" || quantity <= 0 || quantity > MaxQuantity || !validMoney(price) {
		return Position{}, Fail("VALIDATION_FAILED", "Positive whole shares, bounded price and PAPER_ONLY confirmation required", 422)
	}
	a, err := s.Account(accountID)
	if err != nil {
		return Position{}, err
	}
	for i := range s.Positions {
		p := &s.Positions[i]
		if p.AccountID != accountID || p.Symbol != symbol {
			continue
		}
		if quantity > p.Quantity {
			return Position{}, Fail("QUANTITY_EXCEEDS_HOLDING", "Cannot paper sell more shares than held", 409)
		}
		proceeds := quantity * price
		if a.Cash > MaxMoney-proceeds {
			return Position{}, Fail("VALIDATION_FAILED", "Paper cash bound exceeded", 422)
		}
		costReduction := p.Cost * quantity / p.Quantity
		p.Cost -= costReduction
		p.Quantity -= quantity
		s.Line(now, a, ID(), "HUMAN_PAPER_SELL", proceeds, 0)
		s.Event(now, "HUMAN_PAPER_SELL_RECORDED", accountID, map[string]any{"symbol": symbol, "quantity": quantity, "price_minor": price, "fees": "zero-fee paper test assumption"})
		return *p, nil
	}
	return Position{}, Fail("QUANTITY_EXCEEDS_HOLDING", "No paper position held", 409)
}
func (s *State) Validate() error {
	if s.Mode != "PAPER" || s.Live {
		return errors.New("live/actual mode cannot be persisted")
	}
	for _, l := range s.Lanes {
		if l.State != "DISABLED" {
			return errors.New("live lane enabled")
		}
	}
	for _, a := range s.Accounts {
		if a.Mode != "PAPER" || a.Cash < 0 || a.Reserved < 0 || a.Reserved > a.Cash {
			return errors.New("paper cash invariant violated")
		}
		var reserved int64
		for _, r := range s.Recommendations {
			if r.AccountID == a.ID {
				reserved += r.Reserved
			}
		}
		if reserved != a.Reserved {
			return errors.New("reservation reconciliation failed")
		}
		var cash, held int64
		for _, j := range s.Journal {
			if j.AccountID == a.ID {
				cash += j.CashDelta
				held += j.ReservedDelta
			}
		}
		if cash != a.Cash || held != a.Reserved {
			return errors.New("journal reconciliation failed")
		}
	}
	for _, p := range s.Positions {
		if p.Quantity < 0 || p.Cost < 0 {
			return errors.New("negative position")
		}
	}
	return nil
}
