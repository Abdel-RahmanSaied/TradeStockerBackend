// Package researchworker carries the research parser source and the exact
// interpreter it is tested on, so platform-core runs what was reviewed.
package researchworker

import (
	_ "embed"
	"strings"
)

//go:embed src/tradestocker_research/financials.py
var Financials string

//go:embed .python-version
var pythonVersion string

// PythonVersion is the exact CPython release the research package is tested on.
var PythonVersion = strings.TrimSpace(pythonVersion)
