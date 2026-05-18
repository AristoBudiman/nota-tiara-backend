# Nyalakan semua
up:
	docker-compose up -d

# Matikan semua
down:
	docker-compose down

# Liat log Go (buat liat error simpan nota)
logs:
	docker logs -f nota_tiara_backend

# Build ulang kalau ada perubahan Dockerfile atau .env
build:
	docker-compose up --build -d

# Generate dokumen Swagger API via Docker
swagger:
	docker exec -it nota_tiara_backend swag init --parseDependency --parseInternal