# Portable Makefile: recipes must work under sh (Linux/macOS) and cmd.exe
# (Windows). Unix-only commands are replaced by the node helpers in tools/.

GO     ?= go
GOROOT ?= $(shell $(GO) env GOROOT)
REPLAY := tools/replays/phase1_noop.rts
ECONOMY := tools/replays/phase3_economy.rts
PATHS := tools/replays/phase4_paths.rts
COMBAT := tools/replays/phase5_combat.rts
ERAS := tools/replays/phase6_eras.rts

ifeq ($(OS),Windows_NT)
DETCHECK := bin\detcheck.exe
else
DETCHECK := ./bin/detcheck
endif

.PHONY: all vendor wasm test test-sim test-det test-parity test-client test-func test-e2e dev fmt vet run-det run-server clean

all: wasm

vendor: client/static/wasm_exec.js

client/static/wasm_exec.js:
	node tools/fsutil.mjs copy "$(GOROOT)/lib/wasm/wasm_exec.js" client/static/wasm_exec.js

wasm: client/static/sim.wasm

client/static/sim.wasm: $(wildcard sim/*/*.go) $(wildcard wasm/*.go)
	node tools/fsutil.mjs mkdir client/static
	node tools/crossexec.mjs GOOS=js GOARCH=wasm -- $(GO) build -trimpath -o client/static/sim.wasm ./wasm

test: test-sim test-det test-parity test-client test-func test-e2e

test-sim:
	$(GO) test ./sim/...

test-det: bin/detcheck
	$(DETCHECK) -verify $(REPLAY)
	$(DETCHECK) -verify $(ECONOMY)
	$(DETCHECK) -verify $(PATHS)
	$(DETCHECK) -verify $(COMBAT)
	$(DETCHECK) -verify $(ERAS)

test-parity: wasm bin/detcheck
	node tools/fsutil.mjs mkdir build
	$(DETCHECK) -replay $(REPLAY) > build/native.txt
	node tools/parity/run.mjs $(REPLAY) > build/wasm.txt
	node tools/parity/check.mjs build/native.txt build/wasm.txt
	$(DETCHECK) -replay $(ECONOMY) > build/native.txt
	node tools/parity/run.mjs $(ECONOMY) > build/wasm.txt
	node tools/parity/check.mjs build/native.txt build/wasm.txt
	$(DETCHECK) -replay $(PATHS) > build/native.txt
	node tools/parity/run.mjs $(PATHS) > build/wasm.txt
	node tools/parity/check.mjs build/native.txt build/wasm.txt
	$(DETCHECK) -replay $(COMBAT) > build/native.txt
	node tools/parity/run.mjs $(COMBAT) > build/wasm.txt
	node tools/parity/check.mjs build/native.txt build/wasm.txt
	$(DETCHECK) -replay $(ERAS) > build/native.txt
	node tools/parity/run.mjs $(ERAS) > build/wasm.txt
	node tools/parity/check.mjs build/native.txt build/wasm.txt

test-client: client/node_modules
	npm --prefix client run test

test-func:
	go test ./server/ -count=1

test-e2e: bin/rts-server client/node_modules
	@if [ "$${NO_E2E:-0}" = "1" ]; then echo "e2e skipped (NO_E2E=1)"; else npm --prefix client run e2e; fi

client/node_modules: client/package.json client/package-lock.json
	npm --prefix client ci

run-det: bin/detcheck
	$(DETCHECK) -replay $(REPLAY)

bin/detcheck: $(wildcard tools/detcheck/*.go) $(wildcard sim/*/*.go)
	node tools/fsutil.mjs mkdir bin
	$(GO) build -trimpath -o bin/detcheck ./tools/detcheck

bin/rts-server: server/*.go server/wire/*.go $(wildcard sim/*/*.go)
	go build -o bin/rts-server ./server/cmd

run-server: bin/rts-server
	./bin/rts-server

dev: wasm vendor client/node_modules
	npm --prefix client run dev

fmt:
	gofmt -w sim tools wasm

vet:
	$(GO) vet ./sim/... ./tools/...
	node tools/crossexec.mjs GOOS=js GOARCH=wasm -- $(GO) vet ./wasm

clean:
	node tools/fsutil.mjs rmrf bin build client/node_modules client/dist client/static/sim.wasm client/static/wasm_exec.js
