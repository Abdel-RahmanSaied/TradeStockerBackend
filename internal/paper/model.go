package paper

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

const Build = "local-paper-v1"
const PolicyVersion = "paper-test-v1-not-approved-for-live"
const MaxMoney int64 = 100_000_000_00
const MaxQuantity int64 = 100_000
const Freshness = 5 * time.Minute

type Lane struct {
	ID    string `json:"id"`
	State string `json:"state"`
}
type Account struct {
	ID       string `json:"id"`
	Currency string `json:"currency"`
	Cash     int64  `json:"cash_minor"`
	Reserved int64  `json:"reserved_minor"`
	Mode     string `json:"mode"`
}
type Quote struct {
	Symbol         string    `json:"symbol"`
	Market         string    `json:"market"`
	Price          int64     `json:"price_minor"`
	Currency       string    `json:"currency"`
	AsOf           time.Time `json:"as_of"`
	Source         string    `json:"source"`
	Mode           string    `json:"mode"`
	Assessment     string    `json:"assessment_id"`
	FinancialValid bool      `json:"financial_valid"`
	HistoryValid   bool      `json:"history_valid"`
}
type Recommendation struct {
	ID        string            `json:"id"`
	Symbol    string            `json:"symbol"`
	Lane      string            `json:"lane"`
	AccountID string            `json:"account_id"`
	Status    string            `json:"status"`
	Quantity  int64             `json:"quantity"`
	Entry     int64             `json:"entry_minor"`
	Stop      int64             `json:"stop_minor"`
	Currency  string            `json:"currency"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	Reasons   []string          `json:"reasons"`
	Mode      string            `json:"mode"`
	Versions  map[string]string `json:"versions"`
	Reserved  int64             `json:"reserved_minor"`
}
type Position struct {
	Symbol    string `json:"symbol"`
	Quantity  int64  `json:"quantity"`
	Cost      int64  `json:"cost_minor"`
	Stop      int64  `json:"stop_minor"`
	AccountID string `json:"account_id"`
	Currency  string `json:"currency"`
}
type Audit struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Action   string    `json:"action"`
	EntityID string    `json:"entity_id"`
	Detail   any       `json:"detail"`
	Actor    string    `json:"actor"`
}
type Notification struct {
	ID     string    `json:"id"`
	At     time.Time `json:"at"`
	Status string    `json:"status"`
	Text   string    `json:"text"`
}
type Journal struct {
	ID            string    `json:"id"`
	At            time.Time `json:"at"`
	AccountID     string    `json:"account_id"`
	EntityID      string    `json:"entity_id"`
	CashDelta     int64     `json:"cash_delta_minor"`
	ReservedDelta int64     `json:"reserved_delta_minor"`
	Reason        string    `json:"reason"`
}
type Receipt struct {
	Hash string `json:"hash"`
	Body []byte `json:"body"`
}
type State struct {
	Mode            string             `json:"mode"`
	Live            bool               `json:"live_advisory_enabled"`
	Lanes           []Lane             `json:"lanes"`
	Accounts        []Account          `json:"accounts"`
	Quotes          []Quote            `json:"quotes"`
	Recommendations []Recommendation   `json:"recommendations"`
	Positions       []Position         `json:"positions"`
	Audit           []Audit            `json:"audit"`
	Notifications   []Notification     `json:"notifications"`
	Journal         []Journal          `json:"journal"`
	EntriesPaused   bool               `json:"entries_paused"`
	Build           string             `json:"build"`
	Receipts        map[string]Receipt `json:"-"`
}

// Persisted wraps private receipts so the public dashboard cannot disclose request bodies.
type Persisted struct {
	State    State              `json:"state"`
	Receipts map[string]Receipt `json:"receipts"`
}

func NewState() State {
	return State{Mode: "PAPER", Build: Build, Lanes: []Lane{{"US_SWING", "DISABLED"}, {"EGX_SWING", "DISABLED"}, {"US_INTRADAY", "DISABLED"}, {"EGX_INTRADAY", "DISABLED"}}, Accounts: []Account{}, Quotes: []Quote{}, Recommendations: []Recommendation{}, Positions: []Position{}, Audit: []Audit{}, Notifications: []Notification{}, Journal: []Journal{}, Receipts: map[string]Receipt{}}
}
func ID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func (s *State) Event(now time.Time, action, entity string, detail any) {
	s.Audit = append(s.Audit, Audit{ID: ID(), At: now, Action: action, EntityID: entity, Detail: detail, Actor: "local-owner"})
}
func (s *State) Notify(now time.Time, text string) {
	s.Notifications = append(s.Notifications, Notification{ID: ID(), At: now, Status: "DRY_RUN_NOT_SENT", Text: "[PAPER / SYNTHETIC] " + text})
}
func (s *State) Line(now time.Time, a *Account, entity, reason string, cash, reserved int64) {
	a.Cash += cash
	a.Reserved += reserved
	s.Journal = append(s.Journal, Journal{ID: ID(), At: now, AccountID: a.ID, EntityID: entity, Reason: reason, CashDelta: cash, ReservedDelta: reserved})
}
