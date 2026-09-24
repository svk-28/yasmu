.PHONY: run build test

run:
	go run main.go config.go

build:
	go build -ldflags "-s -w" -o bin/watchdog main.go config.go

# test:
    	# go test ./...# 
