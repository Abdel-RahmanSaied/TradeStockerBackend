package researchworker

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedParserIsThePackageSource(t *testing.T) {
	source, err := os.ReadFile("src/tradestocker_research/financials.py")
	if err != nil || string(source) != Financials {
		t.Fatalf("embedded parser differs from the package source: %v", err)
	}
}

// The Go startup check, the package metadata and the toolchain pin must agree.
func TestPythonPinMatchesThePackage(t *testing.T) {
	if !regexp.MustCompile(`^3\.\d+\.\d+$`).MatchString(PythonVersion) {
		t.Fatalf("pin %q is not an exact CPython release", PythonVersion)
	}
	project, err := os.ReadFile("pyproject.toml")
	if err != nil {
		t.Fatal(err)
	}
	minor := PythonVersion[:strings.LastIndex(PythonVersion, ".")]
	if !strings.Contains(string(project), "\nrequires-python = \">="+minor+"\"\n") {
		t.Fatalf("pyproject requires-python does not start at the pinned %s", minor)
	}
}
