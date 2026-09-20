GO    ?= go
SHELL := /usr/bin/env bash

.PHONY: vendor test test-sim fmt vet clean

test: test-sim

test-sim:
	$(GO) test ./sim/...

fmt:
	gofmt -w sim tools

vet:
	$(GO) vet ./sim/...

vendor: client/static/wasm_exec.js

client/static/wasm_exec.js: | client/static
	cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $@

client/static:
	mkdir -p client/static

clean:
	rm -rf bin build client/node_modules client/dist client/static/sim.wasm client/static/wasm_exec.js
