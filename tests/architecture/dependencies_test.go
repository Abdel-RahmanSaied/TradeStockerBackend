// Package architecture checks the module's dependency direction from source:
// which layer each package belongs to and what it may import. It reads import
// lines only, so it needs no build, network or database.
package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const module = "tradestocker"

type layer string

const (
	domain  layer = "domain"     // pure business rules: no I/O, framework or network
	support layer = "support"    // cross-cutting helpers (telemetry): standard library only
	adapter layer = "adapter"    // database, HTTP, processes and providers
	asset   layer = "asset"      // embedded files only (migrations, parser source)
	entry   layer = "entrypoint" // cmd/*: wires adapters to domain code
	tool    layer = "tool"       // scripts: never imported
	check   layer = "check"      // this package
)

// layers classifies every package. A new package fails the test until it is
// classified here, so its dependency rules are a reviewed choice.
var layers = map[string]layer{
	"internal/paper":       domain,
	"internal/telemetry":   support,
	"internal/natsbus":     adapter,
	"internal/httpapi":     adapter,
	"internal/store":       adapter,
	"internal/research":    adapter,
	"db":                   asset,
	"research":             asset,
	"cmd/platform-core":    entry,
	"cmd/market-ingestion": entry,
	"scripts/paperbench":   tool,
	"tests/architecture":   check,
}

// entrypoints is the closed set of Go binaries. The Python research-worker is the
// third entrypoint (research/pyproject.toml). Advisory roles are code inside
// these processes, never a service each.
var entrypoints = []string{"cmd/market-ingestion", "cmd/platform-core"}

// domainStdlib is all the standard library that domain code may use.
var domainStdlib = set("bytes", "crypto/rand", "crypto/sha256", "encoding/hex", "encoding/json", "errors", "fmt",
	"maps", "math", "math/big", "regexp", "slices", "sort", "strconv", "strings", "time", "unicode", "unicode/utf8")

// assetStdlib is all that an embedded-file package may use.
var assetStdlib = set("embed", "io/fs", "strings")

func set(items ...string) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		out[item] = true
	}
	return out
}

// kind is "module" (with the package path relative to the module root), "std" or "third".
func kind(path string) (string, string) {
	if rel, ok := strings.CutPrefix(path, module+"/"); ok {
		return "module", rel
	}
	if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
		return "third", path
	}
	return "std", path
}

// violations applies the rules to a package graph: package (relative path) to the
// import paths of its non-test files.
func violations(graph map[string][]string) []string {
	var out []string
	bad := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	var cmds []string
	for pkg, imports := range graph {
		from, ok := layers[pkg]
		if !ok {
			bad("%s is not classified in layers", pkg)
			continue
		}
		if strings.HasPrefix(pkg, "cmd/") {
			cmds = append(cmds, pkg)
		}
		for _, imp := range imports {
			k, rel := kind(imp)
			to := layers[rel]
			switch {
			case from == domain && k == "std" && !domainStdlib[rel]:
				bad("domain %s imports %s (I/O, network or framework)", pkg, imp)
			case from == domain && k == "third":
				bad("domain %s imports third-party %s", pkg, imp)
			case from == domain && k == "module" && to != domain:
				bad("domain %s imports %s %s", pkg, to, rel)
			case from == asset && (k != "std" || !assetStdlib[rel]):
				bad("asset %s imports %s", pkg, imp)
			case from == support && k != "std":
				bad("support %s imports %s (only the standard library)", pkg, imp)
			case from == adapter && k == "module" && to != domain && to != asset && to != support:
				bad("adapter %s imports %s %s", pkg, to, rel)
			case k == "module" && (to == entry || to == tool || to == check):
				bad("%s imports %s %s", pkg, to, rel)
			}
		}
	}
	sort.Strings(cmds)
	if strings.Join(cmds, ",") != strings.Join(entrypoints, ",") {
		bad("Go entrypoints are %v, want exactly %v", cmds, entrypoints)
	}
	// market-ingestion stays independent of core authority: it reaches no
	// database, research, NATS or HTTP-server adapter and no third-party code;
	// only domain and support packages.
	seen := map[string]bool{}
	var walk func(string)
	walk = func(pkg string) {
		for _, imp := range graph[pkg] {
			k, rel := kind(imp)
			if k == "third" {
				bad("market-ingestion reaches third-party %s via %s", imp, pkg)
			}
			if k == "module" && !seen[rel] {
				seen[rel] = true
				if layers[rel] != domain && layers[rel] != support {
					bad("market-ingestion reaches %s %s via %s", layers[rel], rel, pkg)
				}
				walk(rel)
			}
		}
	}
	walk("cmd/market-ingestion")
	sort.Strings(out)
	return out
}

// moduleGraph parses the import lines of every non-test Go file under root.
// approvedNestedModules are the only folders that may hold their own go.mod; they
// are separate experiments, not product code, and are not scanned.
var approvedNestedModules = set("spikes/s00-t05/go")

// moduleGraph parses the import lines of every non-test Go file under root and
// also returns any nested module that is not approved. Such a module is still
// scanned, so its packages show up as unclassified too. Hidden folders (tool
// caches), testdata and node_modules are skipped.
func moduleGraph(t *testing.T, root string) (map[string][]string, []string) {
	t.Helper()
	if b, err := os.ReadFile(filepath.Join(root, "go.mod")); err != nil || !strings.HasPrefix(string(b), "module "+module+"\n") {
		t.Fatalf("module root not found at %s", root)
	}
	graph := map[string][]string{}
	var nested []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "node_modules") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); path != root && err == nil {
				rel, _ := filepath.Rel(root, path)
				if approvedNestedModules[filepath.ToSlash(rel)] {
					return filepath.SkipDir
				}
				nested = append(nested, filepath.ToSlash(rel))
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		rel = filepath.ToSlash(rel)
		if _, ok := graph[rel]; !ok {
			graph[rel] = nil
		}
		for _, spec := range file.Imports {
			graph[rel] = append(graph[rel], strings.Trim(spec.Path.Value, `"`))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return graph, nested
}

func TestModuleDependencyDirection(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	graph, nested := moduleGraph(t, root)
	if len(graph) < len(layers)-1 {
		t.Fatalf("found only %d packages: %v", len(graph), graph)
	}
	for _, dir := range nested {
		t.Errorf("unapproved nested Go module %s: product code lives in the one root module", dir)
	}
	for _, v := range violations(graph) {
		t.Error(v)
	}
	for pkg := range layers {
		if _, ok := graph[pkg]; !ok && layers[pkg] != check {
			t.Errorf("classified package %s no longer exists", pkg)
		}
	}
}

// fakeModule writes files into a new folder the test owns and returns it.
func fakeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, text := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The parser path end to end on a fake module the test owns: a domain package
// importing net/http must be reported, and the approved spike module is not read.
func TestSourceScanCatchesARealImport(t *testing.T) {
	root := fakeModule(t, map[string]string{
		"go.mod":                       "module " + module + "\n",
		"internal/paper/model.go":      "package paper\n\nimport _ \"net/http\"\n",
		"internal/paper/model_test.go": "package paper\n\nimport _ \"os/exec\"\n",
		"cmd/platform-core/main.go":    "package main\n",
		"cmd/market-ingestion/main.go": "package main\n\nimport _ \"tradestocker/internal/paper\"\n",
		"spikes/s00-t05/go/go.mod":     "module spike\n",
		"spikes/s00-t05/go/main.go":    "package main\n\nimport _ \"tradestocker/internal/store\"\n",
	})
	graph, nested := moduleGraph(t, root)
	if _, ok := graph["spikes/s00-t05/go"]; ok || len(nested) != 0 {
		t.Fatalf("approved spike module was scanned or reported: %v", nested)
	}
	got := strings.Join(violations(graph), "\n")
	if !strings.Contains(got, "domain internal/paper imports net/http") {
		t.Fatalf("real import not reported: %s", got)
	}
	if strings.Contains(got, "os/exec") {
		t.Fatalf("test-only import was treated as production code: %s", got)
	}
}

// A nested go.mod anywhere else (for example a service per advisory role) must
// not hide its packages from the closed entrypoint set.
func TestUnexpectedNestedModuleIsReported(t *testing.T) {
	for _, dir := range []string{"cmd/technical-analyst", "internal/goals", "spikes/other", "spikes/s00-t05"} {
		t.Run(dir, func(t *testing.T) {
			root := fakeModule(t, map[string]string{
				"go.mod":                       "module " + module + "\n",
				"internal/paper/model.go":      "package paper\n",
				"cmd/platform-core/main.go":    "package main\n",
				"cmd/market-ingestion/main.go": "package main\n",
				dir + "/go.mod":                "module role\n",
				dir + "/main.go":               "package main\n\nimport _ \"net/http\"\n",
			})
			graph, nested := moduleGraph(t, root)
			if len(nested) != 1 || nested[0] != dir {
				t.Fatalf("nested module %s not reported: %v", dir, nested)
			}
			if _, ok := graph[dir]; !ok {
				t.Fatalf("packages of nested module %s were hidden from the graph", dir)
			}
			if !strings.Contains(strings.Join(violations(graph), "\n"), dir+" is not classified") {
				t.Fatalf("nested module %s escaped classification", dir)
			}
		})
	}
}

// Each broken graph must be caught, so a passing real graph means something.
func TestRulesCatchViolations(t *testing.T) {
	good := map[string][]string{
		"internal/paper":       {"errors", "time"},
		"internal/telemetry":   {"log/slog", "net/http"},
		"internal/natsbus":     {"github.com/nats-io/nats.go", "tradestocker/internal/telemetry"},
		"internal/store":       {"tradestocker/internal/paper", "tradestocker/db", "github.com/jackc/pgx/v5"},
		"internal/httpapi":     {"net/http", "tradestocker/internal/paper"},
		"internal/research":    {"os/exec", "tradestocker/research"},
		"db":                   {"embed"},
		"research":             {"embed", "strings"},
		"cmd/platform-core":    {"tradestocker/internal/store", "tradestocker/internal/httpapi"},
		"cmd/market-ingestion": {"net/http", "tradestocker/internal/paper"},
		"scripts/paperbench":   {"net/http"},
	}
	if v := violations(good); len(v) != 0 {
		t.Fatalf("valid graph rejected: %v", v)
	}
	for name, change := range map[string]func(map[string][]string){
		"domain uses network":           func(g map[string][]string) { g["internal/paper"] = append(g["internal/paper"], "net/http") },
		"domain runs processes":         func(g map[string][]string) { g["internal/paper"] = append(g["internal/paper"], "os/exec") },
		"domain uses a database driver": func(g map[string][]string) { g["internal/paper"] = append(g["internal/paper"], "database/sql") },
		"domain uses third-party code": func(g map[string][]string) {
			g["internal/paper"] = append(g["internal/paper"], "github.com/go-chi/chi/v5")
		},
		"domain imports an adapter": func(g map[string][]string) {
			g["internal/paper"] = append(g["internal/paper"], "tradestocker/internal/store")
		},
		"adapter imports an adapter": func(g map[string][]string) {
			g["internal/httpapi"] = append(g["internal/httpapi"], "tradestocker/internal/store")
		},
		"asset gains code": func(g map[string][]string) { g["db"] = append(g["db"], "github.com/jackc/pgx/v5") },
		"package imports an entrypoint": func(g map[string][]string) {
			g["internal/store"] = append(g["internal/store"], "tradestocker/cmd/platform-core")
		},
		"package imports a tool": func(g map[string][]string) {
			g["internal/store"] = append(g["internal/store"], "tradestocker/scripts/paperbench")
		},
		"unclassified package":      func(g map[string][]string) { g["internal/goals"] = []string{"time"} },
		"service per advisory role": func(g map[string][]string) { g["cmd/technical-analyst"] = []string{"tradestocker/internal/paper"} },
		"missing entrypoint":        func(g map[string][]string) { delete(g, "cmd/market-ingestion") },
		"ingestion reaches the database": func(g map[string][]string) {
			g["cmd/market-ingestion"] = append(g["cmd/market-ingestion"], "tradestocker/internal/store")
		},
		"ingestion reaches research": func(g map[string][]string) {
			g["cmd/market-ingestion"] = append(g["cmd/market-ingestion"], "tradestocker/internal/research")
		},
		"ingestion uses third-party": func(g map[string][]string) {
			g["cmd/market-ingestion"] = append(g["cmd/market-ingestion"], "github.com/x/feed")
		},
		"ingestion reaches NATS": func(g map[string][]string) {
			g["cmd/market-ingestion"] = append(g["cmd/market-ingestion"], "tradestocker/internal/natsbus")
		},
		"support uses third-party code": func(g map[string][]string) {
			g["internal/telemetry"] = append(g["internal/telemetry"], "github.com/x/otel")
		},
		"support imports an adapter": func(g map[string][]string) {
			g["internal/telemetry"] = append(g["internal/telemetry"], "tradestocker/internal/store")
		},
		"domain imports support": func(g map[string][]string) {
			g["internal/paper"] = append(g["internal/paper"], "tradestocker/internal/telemetry")
		},
	} {
		t.Run(name, func(t *testing.T) {
			graph := map[string][]string{}
			for k, v := range good {
				graph[k] = append([]string(nil), v...)
			}
			change(graph)
			if v := violations(graph); len(v) == 0 {
				t.Fatal("violation not detected")
			}
		})
	}
}
