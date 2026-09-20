GO     ?= go
SHELL  := /usr/bin/env bash
REPLAY := tools/replays/phase1_noop.rts

.PHONY: vendor test test-sim test-det fmt vet run-det clean

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
	gofmt -w sim tools

vet:
	$(GO) vet ./sim/... ./tools/...

vendor: client/static/wasm_exec.js

client/static/wasm_exec.js: | client/static
	cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $@

client/static:
	mkdir -p client/static

clean:
	rm -rf bin build client/node_modules client/dist client/static/sim.wasm client/static/wasm_exec.js
