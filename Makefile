.PHONY: run build tidy docker-up docker-down deploy

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
deploy:
	go mod vendor
	DOCKER_HOST="unix:///var/folders/yy/n57fmv7x1djdbzg38zs4k4sc0000gq/T/podman/podman-machine-default-api.sock" flyctl deploy --local-only

setup:
	go mod tidy
	cp .env.example .env
	@echo "Edit .env with your DATABASE_URL and VAULT_ENCRYPTION_KEY, then: make run"
