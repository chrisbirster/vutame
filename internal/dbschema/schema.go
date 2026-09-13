package dbschema

import _ "embed"

// SQL is the desired Vutame SQLite schema. Atlas owns applying this schema;
// application startup only verifies that the required tables already exist.
//
//go:embed schema.sql
var SQL string
