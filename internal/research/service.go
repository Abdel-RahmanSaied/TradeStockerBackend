package research

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service runs durable SEC research jobs. It never reads or writes paper financial state.
type Service struct {
	Pool       *pgxpool.Pool
	Provider   Fetcher
	Configured bool
	Python     string // interpreter; default python3
	Script     string // parser source; default the embedded research/financials.py
}

type Job struct {
	ID            string          `json:"id"`
	Symbol        string          `json:"symbol"`
	CIK           string          `json:"cik"`
	Status        string          `json:"status"`
	Attempts      int             `json:"attempts"`
	MaxAttempts   int             `json:"max_attempts"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	NextAttemptAt *time.Time      `json:"next_attempt_at,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	Error         *string         `json:"error_code,omitempty"`
	LastError     *string         `json:"last_error,omitempty"`
}

type Event struct {
	At     time.Time       `json:"at"`
	Action string          `json:"action"`
	Detail json.RawMessage `json:"detail"`
}

const jobColumns = `id,symbol,cik,status,attempts,created_at,updated_at,CASE WHEN status='QUEUED' THEN not_before END,result,error_code,last_error`

func scanJob(row pgx.Row) (Job, error) {
	j := Job{MaxAttempts: maxAttempts}
	err := row.Scan(&j.ID, &j.Symbol, &j.CIK, &j.Status, &j.Attempts, &j.CreatedAt, &j.UpdatedAt, &j.NextAttemptAt, &j.Result, &j.Error, &j.LastError)
	return j, err
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Human-readable messages for request errors. Job failures use provider codes.
var messages = map[string]string{
	"JSON_REQUIRED":                        "Content-Type must be application/json",
	"IDEMPOTENCY_KEY_REQUIRED":             "An 8–128 character Idempotency-Key is required",
	"BODY_LIMIT":                           "Request body exceeds 4 KiB or cannot be read",
	"US_SYMBOL_AND_TEN_DIGIT_CIK_REQUIRED": "Provide exactly a US ticker and a 10-digit SEC CIK",
	"IDEMPOTENCY_CONFLICT":                 "Idempotency key was already used for a different request",
	"SEC_CONTACT_NOT_CONFIGURED":           "SEC contact is not configured; set TS_SEC_USER_AGENT and restart",
	"RESEARCH_QUEUE_LIMIT":                 "Too many research jobs are waiting; retry after they finish",
	"RESEARCH_NOT_FOUND":                   "Research job not found",
	"SOURCE_NOT_FOUND":                     "Original source not found for this job",
	"INVALID_SOURCE_HASH":                  "sha256 must be 64 lowercase hex characters",
	"RESEARCH_STORAGE_UNAVAILABLE":         "Research storage failed; no success is assumed",
	"RESEARCH_ID_FAILED":                   "Could not create a job identifier",
}

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/research/status", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var cooldown *time.Time
		err := s.Pool.QueryRow(ctx, `SELECT not_before FROM research_provider_state WHERE provider='SEC' AND not_before>now()`).Scan(&cooldown)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
			return
		}
		respond(w, http.StatusOK, map[string]any{"configured": s.Configured, "source": "SEC_PUBLIC", "mode": "READ_ONLY_RESEARCH", "parser_version": ParserVersion,
			"max_attempts": maxAttempts, "provider_cooldown_until": cooldown, "live_advisory_enabled": false})
	})
	mux.HandleFunc("POST /api/v1/research/jobs", s.create)
	mux.HandleFunc("GET /api/v1/research/jobs", s.list)
	mux.HandleFunc("GET /api/v1/research/jobs/{id}", s.detail)
	mux.HandleFunc("GET /api/v1/research/jobs/{id}/sources/{kind}", s.source)
	return mux
}

func respond(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, code string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": messages[code]}})
}

// create queues a job. A repeated Idempotency-Key with the same body returns the
// same job (in its current state); with a different body it is a conflict.
func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		fail(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		fail(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REQUIRED")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		fail(w, http.StatusRequestEntityTooLarge, "BODY_LIMIT")
		return
	}
	var in struct {
		Symbol string `json:"symbol"`
		CIK    string `json:"cik"`
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || !symbolPattern.MatchString(in.Symbol) || !cikPattern.MatchString(in.CIK) || in.CIK == "0000000000" {
		fail(w, http.StatusUnprocessableEntity, "US_SYMBOL_AND_TEN_DIGIT_CIK_REQUIRED")
		return
	}
	canonical, _ := json.Marshal(in)
	h := sha256.Sum256(canonical)
	hash := hex.EncodeToString(h[:])
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	defer tx.Rollback(context.Background())
	// Serializes creation so the key check and the queue bound are exact.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(81342103)`); err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	var id, oldHash string
	err = tx.QueryRow(ctx, `SELECT id,request_hash FROM research_jobs WHERE operation_key=$1`, key).Scan(&id, &oldHash)
	if err == nil {
		if oldHash != hash {
			fail(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT")
			return
		}
		j, e := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM research_jobs WHERE id=$1`, id))
		if e != nil {
			fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
			return
		}
		respond(w, http.StatusAccepted, j)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	if !s.Configured {
		fail(w, http.StatusServiceUnavailable, "SEC_CONTACT_NOT_CONFIGURED")
		return
	}
	var active, total int
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status IN ('QUEUED','RUNNING')),count(*) FROM research_jobs`).Scan(&active, &total); err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	if active >= maxActiveJobs || total >= maxStoredJobs {
		fail(w, http.StatusTooManyRequests, "RESEARCH_QUEUE_LIMIT")
		return
	}
	var b [16]byte
	if _, err = rand.Read(b[:]); err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_ID_FAILED")
		return
	}
	id = "research_" + hex.EncodeToString(b[:])
	j, err := scanJob(tx.QueryRow(ctx, `INSERT INTO research_jobs(id,operation_key,request_hash,symbol,cik,status) VALUES($1,$2,$3,$4,$5,'QUEUED') RETURNING `+jobColumns, id, key, hash, in.Symbol, in.CIK))
	if err == nil {
		err = addEvent(ctx, tx, id, "REQUESTED", nil)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	respond(w, http.StatusAccepted, j)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	// Results are omitted from the list; read one job for its report.
	rows, err := s.Pool.Query(ctx, `SELECT id,symbol,cik,status,attempts,created_at,updated_at,CASE WHEN status='QUEUED' THEN not_before END,NULL::jsonb,error_code,last_error FROM research_jobs ORDER BY created_at DESC, id LIMIT 50`)
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	jobs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Job, error) { return scanJob(row) })
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	if jobs == nil {
		jobs = []Job{}
	}
	respond(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Service) detail(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	j, err := scanJob(s.Pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM research_jobs WHERE id=$1`, r.PathValue("id")))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "RESEARCH_NOT_FOUND")
		return
	}
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	rows, err := s.Pool.Query(ctx, `SELECT attempt,kind,sha256,url,received_at,octet_length(body) FROM research_sources WHERE job_id=$1 ORDER BY attempt,kind`, j.ID)
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	sources, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Source, error) {
		var src Source
		err := row.Scan(&src.Attempt, &src.Kind, &src.SHA256, &src.URL, &src.ReceivedAt, &src.Size)
		return src, err
	})
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	rows, err = s.Pool.Query(ctx, `SELECT at,action,detail FROM research_events WHERE job_id=$1 ORDER BY sequence`, j.ID)
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
		var e Event
		err := row.Scan(&e.At, &e.Action, &e.Detail)
		return e, err
	})
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	if sources == nil {
		sources = []Source{}
	}
	respond(w, http.StatusOK, map[string]any{"job": j, "sources": sources, "events": events})
}

// source returns the exact original bytes. With ?sha256= only a matching copy is served.
func (s *Service) source(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != "companyfacts" && kind != "submissions" {
		fail(w, http.StatusNotFound, "SOURCE_NOT_FOUND")
		return
	}
	want := r.URL.Query().Get("sha256")
	if want != "" && !sha256Pattern.MatchString(want) {
		fail(w, http.StatusUnprocessableEntity, "INVALID_SOURCE_HASH")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var body []byte
	err := s.Pool.QueryRow(ctx, `SELECT body FROM research_sources WHERE job_id=$1 AND kind=$2 AND ($3='' OR sha256=$3) ORDER BY attempt DESC LIMIT 1`, r.PathValue("id"), kind, want).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "SOURCE_NOT_FOUND")
		return
	}
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "RESEARCH_STORAGE_UNAVAILABLE")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+kind+`.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func addEvent(ctx context.Context, tx pgx.Tx, jobID, action string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO research_events(job_id,action,detail) VALUES($1,$2,$3)`, jobID, action, b)
	return err
}
