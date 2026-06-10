.PHONY: build run test lint

build:
	go build -o bin/bot ./cmd/bot

run:
	./bin/bot

test:
	go test ./...

lint:
	go vet ./...

publish:
	docker build . -t devsteam/slack-query-executor:distroless
	docker push devsteam/slack-query-executor:distroless

