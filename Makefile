GO    ?= go
SHELL := /usr/bin/env bash

.PHONY: vendor clean

vendor: client/static/wasm_exec.js

client/static/wasm_exec.js: | client/static
	cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $@

client/static:
	mkdir -p client/static

clean:
	rm -rf bin build client/node_modules client/dist client/static/sim.wasm client/static/wasm_exec.js
