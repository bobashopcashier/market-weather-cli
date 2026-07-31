BINS := mwx dataframe betmoar metar wethr polyweather open-meteo meteoblue wunderground
DIST := dist

.PHONY: all build test check install clean

all: check build

build:
	@mkdir -p $(DIST)
	@for bin in $(BINS); do \
		go build -trimpath -o $(DIST)/$$bin ./cmd/$$bin; \
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
	rm -rf $(DIST)
