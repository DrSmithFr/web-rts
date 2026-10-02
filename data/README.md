# data

Balance data for units, buildings and technologies as readable files (JSON,
embedded at build time with `go:embed`, parsed once at startup, never on the
simulation hot path).

Unit and building stat schemas land in phase 3 together with the first
collectable resources and buildings.

## units.json: behaviour fields

Values are integers (fixed point where a distance is involved, see
`sim/fixed`). Rules live in `sim/core`, parsing and validation in
`sim/units/units.go`.

| Field | Meaning |
| --- | --- |
| `turnRate` | Degrees per tick the actor can pivot; free movement follows `heading`, never a grid step. Default 30 (`DefaultTurnRate`), valid 1..359. |
| `vision` | Radius in tiles of the sight cone. |
| `visionArc` | Field of view in degrees; 360 sees all around. Default 360 (`DefaultVisionArc`), valid 1..360. A sleeping unit sees nothing, and fog of war only reveals inside the cone. |
| `terrorTicks` | How long an entity that was struck runs in panic. Only species that can panic carry one (0 = never panics). |
| `preyPreferred` | Neutral animal def ids a hunter takes the moment it sees them. |
| `preyFallback` | Opportunistic meals, chased only while the hunger gate is open. |
| `preyFallbackAt` | Hunger percentage (1..100) at or below which `preyFallback` comes into play. Required when `preyFallback` is set. |
| `hungerDecay` | Ticks a hunter burns per point of food, counted from 100 (sated) down to 0 (starving). Only hunters (a prey list) may set it. |
| `rageSpeed`, `rageAggro` | Percent multipliers on speed and aggro radius while enraged (food at or below 10). |
| `corpseFood` | Food a carcass yields the hunter that eats it. |

A species listed on any hunter's prey menu is flagged `Hunted` at load time
(derived, not in JSON): it is prey, so fear and terror apply to it.
Bold (food at or below 20) switches off avoidance: campfire auras,
settlement radii, escorted prey and the fallback gate.
