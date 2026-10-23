// Package contractdocs embeds the contract documents (events/, research/ and
// openapi/), so Go code can build an internal/contracts Registry without
// reading files. Fixtures, tools and the compatibility lock are not embedded.
package contractdocs

import "embed"

// FS holds the contract documents at their paths inside contracts/.
//
//go:embed events research openapi
var FS embed.FS
