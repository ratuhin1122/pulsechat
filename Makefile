.DEFAULT_GOAL := help

COMPOSE = docker compose -f docker-compose.yml -f deploy/compose.local.yml
PROD_COMPOSE = docker compose -f docker-compose.yml

.PHONY: help check start up stop down restart logs ps migrate build prod-start prod-stop

help:
	@echo make start      - Build, migrate, and start locally at http://localhost:8080
	@echo make stop       - Stop local containers and keep data
	@echo make logs       - Follow local service logs
	@echo make ps         - Show local service status
	@echo make migrate    - Apply pending database migrations
	@echo make build      - Build local images
	@echo make prod-start - Start the HTTPS deployment using .env

check:
	docker info --format "Docker engine: {{.ServerVersion}}"
	$(COMPOSE) config --quiet

start up: check
	$(COMPOSE) up -d postgres redis clamav
	$(COMPOSE) run --rm migrate
	$(COMPOSE) up -d --build --wait --wait-timeout 600 api worker caddy
	@echo Pulsechat is ready: http://localhost:8080

stop down:
	$(COMPOSE) down

restart: stop start

logs:
	$(COMPOSE) logs -f --tail=100

ps:
	$(COMPOSE) ps

migrate: check
	$(COMPOSE) up -d postgres
	$(COMPOSE) run --rm migrate

build: check
	$(COMPOSE) build

prod-start:
	$(PROD_COMPOSE) up -d postgres redis clamav
	$(PROD_COMPOSE) run --rm migrate
	$(PROD_COMPOSE) up -d --build --wait --wait-timeout 600

prod-stop:
	$(PROD_COMPOSE) down
