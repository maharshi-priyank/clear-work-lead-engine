.PHONY: run build tidy docker-up docker-down

run:
	go run ./cmd/server

build:
	go build -o bin/leads-engine ./cmd/server

tidy:
	go mod tidy

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

# Install dependencies and run
setup:
	go mod tidy
	cp .env.example .env
	@echo "Edit .env with your DATABASE_URL and VAULT_ENCRYPTION_KEY, then: make run"
