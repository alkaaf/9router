#!/bin/bash
set -e
# Start PostgreSQL for testing
docker compose -f docker-compose.test.yml up -d
# Wait for health check
until docker compose -f docker-compose.test.yml exec -T postgres pg_isready -U test -d 9router_test; do
  echo "Waiting for PostgreSQL..."
  sleep 1
done
echo "PostgreSQL is ready!"
export POSTGRES_URL="postgres://test:test@localhost:5432/9router_test"
