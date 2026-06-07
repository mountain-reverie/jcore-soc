# Go VHDL frontend + socgen test targets.
# `make test` is MEMORY-SAFE by default: when systemd-run is available it runs
# the tests inside a transient cgroup with a hard MemoryMax so a runaway is
# OOM-killed without taking down the machine; it always passes a tight -timeout.
SOCGEN_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
JCORE_SOC_ROOT ?= $(abspath $(SOCGEN_DIR)/../..)
GO ?= go
# Use `env` so the var survives being prefixed by timeout/systemd-run (timeout
# would otherwise try to exec "JCORE_SOC_ROOT=..." as the command).
GOTEST := env JCORE_SOC_ROOT=$(JCORE_SOC_ROOT) $(GO) test ./... -timeout 90s -count=1 -p 1
MEMMAX ?= 2G

.PHONY: test test-unsafe build vet
build:
	cd $(SOCGEN_DIR) && $(GO) build ./...
vet:
	cd $(SOCGEN_DIR) && $(GO) vet ./...
# Memory-capped test run (preferred).
test:
	cd $(SOCGEN_DIR) && \
	if command -v systemd-run >/dev/null 2>&1; then \
		systemd-run --user --scope -p MemoryMax=$(MEMMAX) -p MemorySwapMax=0 -- \
			timeout -s KILL 240 $(GOTEST); \
	else \
		echo "WARNING: systemd-run not found; running with -timeout only (no hard memory cap)"; \
		timeout -s KILL 240 $(GOTEST); \
	fi
# Escape hatch with no cap (use only if you understand the risk).
test-unsafe:
	cd $(SOCGEN_DIR) && $(GOTEST)
