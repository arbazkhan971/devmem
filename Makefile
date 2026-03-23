.PHONY: build test run clean bench benchmark

build:
	go build -o bin/devmem ./cmd/devmem

test:
	go test ./... -v -count=1

run: build
	./bin/devmem

clean:
	rm -rf bin/

bench:
	go build -o bin/devmem-bench ./cmd/devmem-bench

benchmark: bench
	./bin/devmem-bench -v
