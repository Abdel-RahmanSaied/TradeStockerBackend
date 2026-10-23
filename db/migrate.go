package db

import _ "embed"

//go:embed migrations/001_paper_state.sql
var Migration string

//go:embed migrations/002_research.sql
var ResearchMigration string
