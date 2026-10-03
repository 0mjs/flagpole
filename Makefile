.PHONY: up down app api worker web demo seed migrate test prove lint sqlc types build

up: ## Start Postgres, Redis, Redpanda and Mailpit
	docker compose up -d --wait

down:
	docker compose --profile app down

app: ## Run everything in containers; the web app is on :3000
	docker compose --profile app up -d --build --wait

api: ## Run the API on :8080 (migrates on start in development)
	cd api && go run ./cmd/api

worker: ## Run the outbox relay and Kafka consumer
	cd api && go run ./cmd/worker

web: ## Run the web app on :5173
	cd web && npm run dev

demo: ## Run the demo shop on :5174 (make seed writes its SDK key)
	cd demo && npm run dev

seed: ## Empty the database and load the demo data
	cd api && go run ./cmd/flagpole migrate reset && go run ./cmd/flagpole seed -demo-env ../demo/.env.local
	@echo "The schema was recreated: restart make api and make worker if they're running."

migrate:
	cd api && go run ./cmd/flagpole migrate up

test: ## End-to-end tests against the running containers
	cd api && go test -race ./...

prove: ## Walk the running stack through a reviewed change, end to end
	scripts/prove.sh

lint:
	cd api && go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@latest ./...

sqlc: ## Regenerate internal/store from db/queries
	docker run --rm -v $(CURDIR)/api:/src -w /src sqlc/sqlc:1.29.0 generate

types: ## Regenerate the web app's API types from a running API
	cd web && npm run api:types

build:
	cd api && go build ./...
	cd web && npm run build
