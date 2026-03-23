#!/bin/bash
cd /Users/arbaz/devmem

# Gate: build must succeed
if ! go build -o /dev/null ./cmd/devmem 2>/dev/null; then
    echo "0"
    exit 0
fi

# Gate: all tests must pass
TEST_OUTPUT=$(go test ./... -count=1 2>&1)
if echo "$TEST_OUTPUT" | grep -q "^FAIL"; then
    echo "0"
    exit 0
fi

PASS_COUNT=$(go test ./... -v -count=1 2>&1 | grep -c "^--- PASS")
SRC_LINES=$(find internal/ cmd/ -name "*.go" ! -name "*_test.go" -exec cat {} + | wc -l | tr -d ' ')

# Metric: (pass_count * 100) / src_lines
python3 -c "print(round(($PASS_COUNT * 100) / $SRC_LINES, 2))"
