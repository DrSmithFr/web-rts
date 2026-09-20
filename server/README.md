# server

Authoritative multiplayer server in Go: lobby service, invite codes, WebSocket
transport, order relay with `T+2` stamping, periodic hash arbitration,
disconnect/reconnect via snapshot plus order replay, and bot hosting.

Scope starts in phase 7. The first native consumer of `sim/` is
`tools/detcheck`; the server will consume the very same packages.
