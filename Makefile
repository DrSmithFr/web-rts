# Portable Makefile: recipes must work under sh (Linux/macOS) and cmd.exe
# (Windows). Unix-only commands are replaced by the node helpers in tools/.

GO     ?= go
GOROOT ?= $(shell $(GO) env GOROOT)
REPLAY := tools/replays/phase1_noop.rts

ifeq ($(OS),Windows_NT)
DETCHECK := bin\detcheck.exe
else
DETCHECK := ./bin/detcheck
endif

.PHONY: all vendor wasm test test-sim test-det test-parity test-client test-func test-e2e dev fmt vet run-det clean

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

test-parity: wasm bin/detcheck
	node tools/fsutil.mjs mkdir build
	$(DETCHECK) -replay $(REPLAY) > build/native.txt
	node tools/parity/run.mjs $(REPLAY) > build/wasm.txt
	node tools/parity/check.mjs build/native.txt build/wasm.txt

test-client: client/node_modules
	npm --prefix client run test

test-func:
	@echo func tests: websocket scenarios land in phase 7, skipped

test-e2e:
	@echo e2e tests: playwright lands in phase 8, skipped

client/node_modules: client/package.json client/package-lock.json
	npm --prefix client ci

run-det: bin/detcheck
	$(DETCHECK) -replay $(REPLAY)

bin/detcheck: $(wildcard tools/detcheck/*.go) $(wildcard sim/*/*.go)
	node tools/fsutil.mjs mkdir bin
	$(GO) build -trimpath -o bin/detcheck ./tools/detcheck

dev: wasm vendor client/node_modules
	npm --prefix client run dev

fmt:
	gofmt -w sim tools wasm

vet:
	$(GO) vet ./sim/... ./tools/...
	node tools/crossexec.mjs GOOS=js GOARCH=wasm -- $(GO) vet ./wasm

clean:
	node tools/fsutil.mjs rmrf bin build client/node_modules client/dist client/static/sim.wasm client/static/wasm_exec.js
