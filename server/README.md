# server

Authoritative multiplayer host in Go (`cmd/main.go`, library in package
`rts/server`).

## What is implemented (phase 7)

- **Lobbies**: rooms created with a 6-character invitation code, up to
  `core.MaxPlayers` slots shared by humans and bot slots, host-only start.
  Bots fill trailing slots and simply never send orders (bot brains land
  in phase 8).
- **WebSocket transport** (`/ws`, JSON frames; order payloads are the
  canonical `sim/orders` binary, base64-encoded). The browser session
  client lives in `client/src/net/session.ts`.
- **Lockstep relay**: the server stamps every received order with a due
  tick of `current + 2` (`sim/lockstep`) and broadcasts the exact order
  batch it stepped with, plus the authoritative state hash, every tick.
  Clients step their own deterministic copy from those batches and
  confirm the hash back.
- **Arbitration**: a client hash disagreeing with the server's marks that
  player desynced and broadcasts a `desync` event.
- **Latency pause**: a connected player falling four hash windows behind
  pauses the room; the game resumes when they catch up.
- **Reconnect**: clients hold a token from `welcome`; rejoining with it
  reclaims the slot and receives the seed plus the full order log for a
  deterministic replay catch-up.
- Seeds and state hashes travel as strings (hex) because they exceed the
  exact-integer range of JSON numbers.

## Running

    make run-server            # listens on :8787
    ./bin/rts-server -addr :8788 -fast   # 50x accelerated tick loop

## Client

Open the game with `?ws=ws://localhost:8787&create=1&name=...&bots=N` to
create a room (add `&autostart=1` or `&startafter=MS` to start without a
lobby UI), or `?ws=ws://localhost:8787&code=XXXXXX` to join; the token in
`sessionStorage` reconnects automatically.

## Tests

    go test ./server/          # functional WebSocket scenarios (-race clean)
    make test-func

Functional scenarios: full lobby cycle, T+2 order-relay discipline,
induced desync detection, reconnect with order-replay catch-up, and a
scripted game against three bots driven to relic victory at accelerated
speed.
