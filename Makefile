.PHONY: test cover lint vuln examples

test:
	go test -race ./...

cover:
	go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run

vuln:
	govulncheck ./...

# Runs both examples against the live service.
examples:
	go run ./examples/basic
	go run ./examples/paging
