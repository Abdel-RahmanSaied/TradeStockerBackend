package contracts

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"testing"

	contractdocs "tradestocker/contracts"
)

// contractsDir is the repository's contracts/ folder, read by tests only.
var contractsDir = os.DirFS("../../contracts")

func eventFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("../../contracts/events/v1")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, "events/v1/"+e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func eventRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry(contractsDir, eventFiles(t)...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestEventSchemasFollowTheDialect(t *testing.T) {
	r := eventRegistry(t)
	for _, name := range r.Names() {
		if problems := r.CheckDialect(name, EventProfile); len(problems) > 0 {
			t.Errorf("%s:\n  %s", name, strings.Join(problems, "\n  "))
		}
	}
}

// The catalog name in CONTRACTS.md (research.assessment.completed.v1) is the
// event_type (research.assessment.completed) plus ".v" plus schema_version; the
// schema file is named after it. Every event lists exactly the envelope fields.
func TestEventSchemasFollowTheNamingRuleAndTheEnvelope(t *testing.T) {
	r := eventRegistry(t)
	envelope, _ := r.Document("events/v1/envelope.schema.json")
	envProps := envelope.(map[string]any)["properties"].(map[string]any)
	envRequired := envelope.(map[string]any)["required"].([]any)
	count := 0
	for _, name := range r.Names() {
		if !strings.HasSuffix(name, ".v1.schema.json") {
			continue
		}
		count++
		doc, _ := r.Document(name)
		s := doc.(map[string]any)
		props := s["properties"].(map[string]any)
		eventType := props["event_type"].(map[string]any)["const"].(string)
		if want := strings.TrimSuffix(path.Base(name), ".schema.json"); eventType+".v1" != want || s["x-tradestocker-catalog-name"] != want {
			t.Errorf("%s: event_type %q and catalog name %v do not follow the file name", name, eventType, s["x-tradestocker-catalog-name"])
		}
		if len(props) != len(envProps) || !equal(s["required"], any(envRequired)) {
			t.Errorf("%s: properties or required differ from the envelope", name)
		}
		for k := range envProps {
			p, ok := props[k].(map[string]any)
			if !ok {
				t.Errorf("%s: missing envelope field %s", name, k)
				continue
			}
			ref, _ := p["$ref"].(string)
			switch {
			case k == "data":
				if ref != "#/$defs/data" {
					t.Errorf("%s: data must reference its own $defs/data", name)
				}
			case k == "account_id" && ref == "common.schema.json#/$defs/id":
				// Account-scoped events require the account.
			case ref != "envelope.schema.json#/properties/"+k:
				t.Errorf("%s: %s must reference the envelope definition, got %q", name, k, ref)
			}
		}
		if s["x-tradestocker-producer"] != props["producer"].(map[string]any)["const"] {
			t.Errorf("%s: producer const and annotation differ", name)
		}
	}
	if count != 13 {
		t.Fatalf("%d event schemas, want the 13 of the CONTRACTS.md catalog", count)
	}
}

// The embedded documents are the files on disk, and a registry built from them
// decodes the golden events.
func TestEmbeddedDocumentsAreTheContractFiles(t *testing.T) {
	var names []string
	err := fs.WalkDir(contractdocs.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, p)
		}
		return err
	})
	if err != nil || len(names) < 18 {
		t.Fatalf("%d embedded documents: %v", len(names), err)
	}
	for _, name := range names {
		embedded, _ := fs.ReadFile(contractdocs.FS, name)
		onDisk, err := os.ReadFile("../../contracts/" + name)
		if err != nil || !bytes.Equal(embedded, onDisk) {
			t.Fatalf("%s differs from the file on disk: %v", name, err)
		}
	}
	r, err := NewRegistry(contractdocs.FS, names...)
	if err != nil {
		t.Fatal(err)
	}
	for eventType, raw := range goldenEvents(t) {
		if _, err := DecodeEvent(r, raw); err != nil {
			t.Errorf("%s: %v", eventType, err)
		}
	}
}
