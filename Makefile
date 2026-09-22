.PHONY: run build test

run:
	go run main.go config-yaml.go

build:
	go build -ldflags "-s -w" -o bin/watchdog main.go config-yaml.go

# test:
    	# go test ./...# 
