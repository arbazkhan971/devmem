.PHONY: build test run clean bench benchmark

build:
	go build -o bin/memorx ./cmd/devmem

test:
	go test ./... -v -count=1

run: build
	./bin/memorx

clean:
	rm -rf bin/

bench:
	go build -o bin/memorx-bench ./cmd/devmem-bench

benchmark: bench
	./bin/memorx-bench -v
