// Package researchworker carries the research parser source and the exact
// interpreter it is tested on, so platform-core runs what was reviewed.
package researchworker

import (
	_ "embed"
	"strings"
)

//go:embed src/tradestocker_research/financials.py
var Financials string

// Telemetry is the trace and safe-event module the launcher gives the parser.
//
//go:embed src/tradestocker_research/telemetry.py
var Telemetry string

// Launcher runs Financials unchanged and logs one safe event in the job's
// trace (see parser_launcher.py for how platform-core composes it).
//
//go:embed parser_launcher.py
var Launcher string

//go:embed .python-version
var pythonVersion string

// PythonVersion is the exact CPython release the research package is tested on.
var PythonVersion = strings.TrimSpace(pythonVersion)
