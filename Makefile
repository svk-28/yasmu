.PHONY: run build static test

run:
	go run main.go config.go

build:
	go build -ldflags "-s -w" -o bin/yasmu main.go config.go

static:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/yasmu-static  main.go config.go

# test:
    	# go test ./...# 
