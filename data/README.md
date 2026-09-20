# data

Balance data for units, buildings and technologies as readable files (JSON,
embedded at build time with `go:embed`, parsed once at startup, never on the
simulation hot path).

Unit and building stat schemas land in phase 3 together with the first
collectable resources and buildings.
