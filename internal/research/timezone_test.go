package research

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"tradestocker/internal/contracts"
)

// pgRow is a database row as pgx returns it; Scan copies each value into its destination.
type pgRow []any

func (r pgRow) Scan(dest ...any) error {
	for i, d := range dest {
		reflect.ValueOf(d).Elem().Set(reflect.ValueOf(r[i]))
	}
	return nil
}

// pgx reads timestamptz in the server's local zone (time.Local). An independent
// run on a fresh database served "+03:00" and broke the UTC contract at
// /created_at; every research time read from PostgreSQL is now served in UTC.
func TestResearchTimesFromPostgreSQLAreServedInUTC(t *testing.T) {
	api, err := contracts.NewRegistry(os.DirFS("../../contracts"), "openapi/local-api.v1.json",
		"research/sec-annual-v2.report.schema.json", "research/sec-annual-v2.request.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	local := time.FixedZone("server", 3*60*60)
	at := time.Date(2026, 10, 23, 18, 19, 18, 739373000, local)
	next, cooldown := at.Add(30*time.Second), at.Add(10*time.Minute)
	trace := "4bf92f3577b34da6a3ce929d0e0e4736"
	code := "SEC_ACCESS_LIMITED"
	src := fixtureSources()[1]

	job, err := scanJob(pgRow{"research_0123456789abcdef0123456789abcdef", "TEST", "0000000001", "QUEUED", 2, at, at, &next,
		json.RawMessage(nil), (*string)(nil), &code, &trace})
	if err != nil {
		t.Fatal(err)
	}
	source, err := scanSource(pgRow{2, src.Kind, src.SHA256, src.URL, at, int64(10)})
	if err != nil {
		t.Fatal(err)
	}
	// jsonb text as PostgreSQL returns it, written before the conversion.
	event, err := scanEvent(pgRow{at, "PROVIDER_COOLDOWN_STARTED",
		json.RawMessage(`{"code": "SEC_ACCESS_LIMITED", "until": "2026-10-23T18:29:18.739373+03:00", "provider": "SEC"}`)})
	if err != nil {
		t.Fatal(err)
	}

	for name, got := range map[string]time.Time{"created_at": job.CreatedAt, "updated_at": job.UpdatedAt, "next_attempt_at": *job.NextAttemptAt,
		"received_at": source.ReceivedAt, "event at": event.At, "cooldown": *utcPtr(&cooldown)} {
		if got.Location() != time.UTC {
			t.Errorf("%s is in %v, want UTC", name, got.Location())
		}
	}
	if !job.CreatedAt.Equal(at) || !job.NextAttemptAt.Equal(next) || !source.ReceivedAt.Equal(at) || !event.At.Equal(at) || !utcPtr(&cooldown).Equal(cooldown) {
		t.Error("converting to UTC changed an instant")
	}
	var detail map[string]string
	if err := json.Unmarshal(event.Detail, &detail); err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"code": "SEC_ACCESS_LIMITED", "until": "2026-10-23T15:29:18.739373Z", "provider": "SEC"}; !reflect.DeepEqual(detail, want) {
		t.Errorf("cooldown detail = %v, want %v", detail, want)
	}

	listed := job
	listed.Result = nil
	payloads := map[string]struct {
		ref   string
		value any
	}{
		"job":    {"Job", job},
		"list":   {"JobList", map[string]any{"jobs": []Job{listed}}},
		"detail": {"JobDetail", map[string]any{"job": job, "sources": []Source{source}, "events": []Event{event}}},
		"status": {"ResearchStatus", map[string]any{"configured": true, "source": "SEC_PUBLIC", "mode": "READ_ONLY_RESEARCH", "parser_version": ParserVersion,
			"max_attempts": maxAttempts, "provider_cooldown_until": utcPtr(&cooldown), "live_advisory_enabled": false}},
		// The answer as it was before the conversion is refused, so this test sees the fault.
		"unconverted job": {"Job", Job{ID: job.ID, Symbol: "TEST", CIK: "0000000001", Status: "QUEUED", Attempts: 2, MaxAttempts: maxAttempts,
			CreatedAt: at, UpdatedAt: at}},
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
		violation := api.Validate("openapi/local-api.v1.json#/components/schemas/"+p.ref, v)
		if name == "unconverted job" {
			if violation == nil || violation.Path != "/created_at" {
				t.Errorf("a %v time was not refused at /created_at: %v", local, violation)
			}
		} else if violation != nil {
			t.Errorf("%s breaks its contract: %v\n%s", name, violation, raw)
		}
	}
}

func TestStoredEventDetailIsServedWithAUTCUntil(t *testing.T) {
	for _, c := range []struct{ name, stored, want string }{
		{"local offset", `{"provider": "SEC", "until": "2026-10-23T18:29:18.7+03:00"}`, `{"provider":"SEC","until":"2026-10-23T15:29:18.7Z"}`},
		{"already UTC", `{"until": "2026-10-23T15:29:18Z"}`, `{"until":"2026-10-23T15:29:18Z"}`},
	} {
		if got := string(utcDetail(json.RawMessage(c.stored))); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	// Without a readable "until" the stored bytes are served unchanged.
	for _, stored := range []string{`{"attempt": 1}`, `{"until": "soon"}`, `{"until": null}`, `null`, `{}`} {
		if got := string(utcDetail(json.RawMessage(stored))); got != stored {
			t.Errorf("%s was changed to %s", stored, got)
		}
	}
}
