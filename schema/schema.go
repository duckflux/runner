package schema

import _ "embed"

// JSON is the embedded duckflux JSON Schema.
//
//go:embed duckflux.schema.json
var JSON []byte
