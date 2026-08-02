BINS := mwx betmoar metar wethr polyweather open-meteo meteoblue wunderground
BUILD_DIR := $(CURDIR)/dist

.PHONY: all build fmt-check vet test race check install clean

all: check build

build:
	@rm -rf -- "$(BUILD_DIR)"
	@mkdir -p "$(BUILD_DIR)"
	@for bin in $(BINS); do \
		go build -trimpath -o "$(BUILD_DIR)/$$bin" ./cmd/$$bin; \
	done

test:
	go test ./...

fmt-check:
	test -z "$$(gofmt -l cmd internal)"

vet:
	go vet ./...

race:
	go test -race ./...

check: fmt-check vet test race

install:
	@for bin in $(BINS); do \
		go install ./cmd/$$bin; \
	done

clean:
	rm -rf -- "$(BUILD_DIR)"
