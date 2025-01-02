run:
	@go run .

build:
	@go build -o ./bin/app .

lint:
	@golangci-lint run

