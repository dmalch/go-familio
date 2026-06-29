.PHONY: build vet test test-acceptance lint check

build:
	go build ./...

vet:
	go vet ./...

# Unit tests (no network). The live settlement-persons decode test self-skips
# unless FAMILIO_NETWORK_TEST=1 is set.
test:
	go test ./...

# Runs the live decode test against the real familio.org API (a read-only
# smoke check of the wire types against production data). Self-skips when
# FAMILIO_NETWORK_TEST is unset; CI never runs it.
test-acceptance:
	FAMILIO_NETWORK_TEST=1 go test -v -count=1 ./...

lint:
	golangci-lint run ./...

# `make check` runs the same gates CI runs. Equivalent to a manual pre-push.
check: build vet lint test
