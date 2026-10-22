package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"tradestocker/internal/paper"
)

type Executor interface {
	Execute(context.Context, string, string, func(*paper.State, time.Time) (any, error)) ([]byte, error)
}
type API struct {
	Store Executor
	Token string
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", a.handle(func(s *paper.State, _ *http.Request, _ []byte, _ time.Time) (any, error) {
		return map[string]any{"status": "ok", "storage": "postgresql", "mode": "PAPER", "live_advisory_enabled": false}, nil
	}))
	mux.HandleFunc("GET /api/v1/dashboard", a.handle(func(s *paper.State, _ *http.Request, _ []byte, _ time.Time) (any, error) { return s, nil }))
	mux.HandleFunc("GET /api/v1/risk/policy", a.handle(func(s *paper.State, _ *http.Request, _ []byte, _ time.Time) (any, error) {
		return map[string]any{"mode": "PAPER", "approval": "DEV_PLACEHOLDER_NOT_APPROVED", "version": paper.PolicyVersion, "data_requirement": "SYNTHETIC_ONLY", "max_trade_risk_bps": 100, "max_total_open_risk_bps": 300, "max_symbol_exposure_bps": 2000, "freshness_seconds": 300, "live_advisory_enabled": false, "fees": "zero-fee test assumption"}, nil
	}))
	mux.HandleFunc("POST /api/v1/fixtures", a.handle(func(s *paper.State, _ *http.Request, b []byte, n time.Time) (any, error) {
		if e := decode(b, &struct{}{}); e != nil {
			return nil, e
		}
		return s.LoadFixtures(n), nil
	}))
	mux.HandleFunc("POST /api/v1/paper/accounts", a.handle(func(s *paper.State, _ *http.Request, b []byte, n time.Time) (any, error) {
		var in struct {
			Currency string `json:"currency"`
			Amount   int64  `json:"amount_minor"`
		}
		if e := decode(b, &in); e != nil {
			return nil, e
		}
		return s.CreateAccount(in.Currency, in.Amount, n)
	}))
	mux.HandleFunc("POST /api/v1/paper/recommendations", a.handle(func(s *paper.State, _ *http.Request, b []byte, n time.Time) (any, error) {
		var in paper.Candidate
		if e := decode(b, &in); e != nil {
			return nil, e
		}
		return s.Evaluate(in, n)
	}))
	mux.HandleFunc("POST /api/v1/paper/recommendations/{id}/acknowledge", a.handle(func(s *paper.State, r *http.Request, b []byte, n time.Time) (any, error) {
		if e := decode(b, &struct{}{}); e != nil {
			return nil, e
		}
		return s.Acknowledge(r.PathValue("id"), n)
	}))
	mux.HandleFunc("POST /api/v1/paper/recommendations/{id}/cancel", a.handle(func(s *paper.State, r *http.Request, b []byte, n time.Time) (any, error) {
		if e := decode(b, &struct{}{}); e != nil {
			return nil, e
		}
		return s.Cancel(r.PathValue("id"), n)
	}))
	mux.HandleFunc("POST /api/v1/paper/recommendations/{id}/fill", a.handle(func(s *paper.State, r *http.Request, b []byte, n time.Time) (any, error) {
		var in struct {
			Price        int64  `json:"price_minor"`
			Confirmation string `json:"confirmation"`
		}
		if e := decode(b, &in); e != nil {
			return nil, e
		}
		return s.Fill(r.PathValue("id"), in.Price, in.Confirmation, n)
	}))
	mux.HandleFunc("POST /api/v1/paper/accounts/{id}/sell", a.handle(func(s *paper.State, r *http.Request, b []byte, n time.Time) (any, error) {
		var in struct {
			Symbol       string `json:"symbol"`
			Quantity     int64  `json:"quantity"`
			Price        int64  `json:"price_minor"`
			Confirmation string `json:"confirmation"`
		}
		if e := decode(b, &in); e != nil {
			return nil, e
		}
		return s.Sell(r.PathValue("id"), in.Symbol, in.Quantity, in.Price, in.Confirmation, n)
	}))
	mux.HandleFunc("POST /api/v1/paper/entries", a.handle(func(s *paper.State, _ *http.Request, b []byte, n time.Time) (any, error) {
		var in struct {
			Paused *bool `json:"paused"`
		}
		if e := decode(b, &in); e != nil {
			return nil, e
		}
		if in.Paused == nil {
			return nil, paper.Fail("VALIDATION_FAILED", "paused must be boolean", 422)
		}
		s.EntriesPaused = *in.Paused
		s.Event(n, "PAPER_ENTRY_PAUSE_CHANGED", "workspace", map[string]bool{"paused": s.EntriesPaused})
		return map[string]any{"mode": "PAPER", "paused": s.EntriesPaused}, nil
	}))
	mux.HandleFunc("POST /api/v1/research/assessments", a.handle(func(s *paper.State, _ *http.Request, b []byte, n time.Time) (any, error) {
		var in struct {
			Symbol string `json:"symbol"`
		}
		if e := decode(b, &in); e != nil {
			return nil, e
		}
		if in.Symbol == "" {
			return nil, paper.Fail("VALIDATION_FAILED", "symbol required", 422)
		}
		result := map[string]any{"mode": "PAPER", "status": "STUB", "outcome": "INSUFFICIENT_DATA", "symbol": in.Symbol, "reason": "No model, financial statement retrieval or licensed historical analysis connected", "writes_financial_state": false}
		s.Event(n, "RESEARCH_STUB_REQUESTED", in.Symbol, result)
		return result, nil
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		// No credentialed CORS. The local frontend uses its same-origin proxy.
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://127.0.0.1:5173" && origin != "http://localhost:5173" {
			writeError(w, paper.Fail("ORIGIN_DENIED", "Origin is not the local dashboard", 403))
			return
		}
		if r.URL.Path != "/api/v1/health" {
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || len(a.Token) < 24 || subtle.ConstantTimeCompare([]byte(provided), []byte(a.Token)) != 1 {
				writeError(w, paper.Fail("UNAUTHORIZED", "Local owner token required", 401))
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

type operation func(*paper.State, *http.Request, []byte, time.Time) (any, error)

func (a *API) handle(fn operation) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		var body []byte
		var key, hash string
		if r.Method == http.MethodPost {
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				writeError(w, paper.Fail("VALIDATION_FAILED", "Content-Type must be application/json", 415))
				return
			}
			key = r.Header.Get("Idempotency-Key")
			if !keyPattern.MatchString(key) {
				writeError(w, paper.Fail("VALIDATION_FAILED", "An 8–128 character Idempotency-Key is required", 422))
				return
			}
			var err error
			body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
			if err != nil {
				writeError(w, paper.Fail("VALIDATION_FAILED", "Request body exceeds limit or cannot be read", 413))
				return
			}
			h := sha256.Sum256(append([]byte(r.Method+" "+r.URL.RequestURI()+"\n"), body...))
			hash = hex.EncodeToString(h[:])
		}
		result, err := a.Store.Execute(ctx, key, hash, func(s *paper.State, n time.Time) (any, error) { return fn(s, r, body, n) })
		if err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(result)
	}
}
func decode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return paper.Fail("VALIDATION_FAILED", "Invalid JSON or unknown fields", 422)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return paper.Fail("VALIDATION_FAILED", "Exactly one JSON object required", 422)
	}
	return nil
}
func writeError(w http.ResponseWriter, err error) {
	var f *paper.Fault
	if !errors.As(err, &f) {
		slog.Error("request failed", "error", err)
		f = &paper.Fault{Code: "INTERNAL_ERROR", Message: "Storage or internal operation failed; no success is assumed", Status: 503}
	}
	w.WriteHeader(f.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": f.Code, "message": f.Message}})
}
