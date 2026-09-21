// Package assets embeds the balance data files read by the simulation.
// Data is parsed once at startup, never on the simulation hot path.
package assets

import _ "embed"

// BuildingsJSON is the raw content of data/buildings.json.
//
//go:embed data/buildings.json
var BuildingsJSON []byte
