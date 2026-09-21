// Package assets embeds the balance data files read by the simulation.
// Data is parsed once at startup, never on the simulation hot path.
package assets

import _ "embed"

// BuildingsJSON is the raw content of data/buildings.json.
//
//go:embed data/buildings.json
var BuildingsJSON []byte

// UnitsJSON is the raw content of data/units.json.
//
//go:embed data/units.json
var UnitsJSON []byte

// ErasJSON is the raw content of data/eras.json.
//
//go:embed data/eras.json
var ErasJSON []byte

// TechsJSON is the raw content of data/techs.json.
//
//go:embed data/techs.json
var TechsJSON []byte
