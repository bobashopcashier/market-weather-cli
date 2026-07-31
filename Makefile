BINS := mwx betmoar metar wethr polyweather open-meteo meteoblue wunderground
BUILD_DIR := $(CURDIR)/dist

.PHONY: all build test check install clean

all: check build

build:
	@rm -rf -- "$(BUILD_DIR)"
	@mkdir -p "$(BUILD_DIR)"
	@for bin in $(BINS); do \
		go build -trimpath -o "$(BUILD_DIR)/$$bin" ./cmd/$$bin; \
	done

test:
	go test ./...

check:
	gofmt -w $$(find cmd internal -name '*.go')
	go vet ./...
	go test ./...

install:
	@for bin in $(BINS); do \
		go install ./cmd/$$bin; \
	done

clean:
	rm -rf -- "$(BUILD_DIR)"
