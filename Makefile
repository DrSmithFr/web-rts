GO     ?= go
SHELL  := /usr/bin/env bash
REPLAY := tools/replays/phase1_noop.rts

.PHONY: all vendor wasm test test-sim test-det fmt vet run-det clean

all: wasm

wasm: client/static/sim.wasm

client/static/sim.wasm: $(wildcard sim/*/*.go) $(wildcard wasm/*.go) | client/static
	GOOS=js GOARCH=wasm $(GO) build -trimpath -o $@ ./wasm

test: test-sim test-det

test-sim:
	$(GO) test ./sim/...

test-det: bin/detcheck
	./bin/detcheck -verify $(REPLAY)

run-det: bin/detcheck
	./bin/detcheck -replay $(REPLAY)

bin/detcheck: $(wildcard tools/detcheck/*.go) $(wildcard sim/*/*.go) | bin
	$(GO) build -trimpath -o $@ ./tools/detcheck

bin:
	mkdir -p bin

fmt:
	gofmt -w sim tools wasm

vet:
	$(GO) vet ./sim/... ./tools/...
	GOOS=js GOARCH=wasm $(GO) vet ./wasm

vendor: client/static/wasm_exec.js

client/static/wasm_exec.js: | client/static
	cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $@

client/static:
	mkdir -p client/static

clean:
	rm -rf bin build client/node_modules client/dist client/static/sim.wasm client/static/wasm_exec.js
