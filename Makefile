run:
	@go run .

build:
	@rm -rf ./bin
	@mkdir ./bin
	@go build -o ./bin/app .
	@cp sample.env ./bin/.env
	@cp backends.json ./bin/backends.json

copy-sample-files: copy-env copy-backends
	

lint:
	@golangci-lint run


