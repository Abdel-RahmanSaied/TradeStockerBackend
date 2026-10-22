package db

import _ "embed"

//go:embed migrations/001_paper_state.sql
var Migration string
