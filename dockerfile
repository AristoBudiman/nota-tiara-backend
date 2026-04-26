# TAHAP 1: Membangun Aplikasi (Menggunakan versi 1.25 sesuai kodemu)
FROM golang:1.25-alpine AS builder
WORKDIR /app

# Salin file modul dan download dependency
COPY go.mod go.sum ./
RUN go mod download

# Salin seluruh kode
COPY . .

# Build aplikasi menjadi file biner matang (Tanpa Air)
RUN CGO_ENABLED=0 GOOS=linux go build -o tiara-api .

# TAHAP 2: Menjalankan Aplikasi (Super Ringan untuk Render)
FROM alpine:latest
WORKDIR /app

# Ambil hasil build matang dari Tahap 1
COPY --from=builder /app/tiara-api .

# Jalankan aplikasi secara langsung
CMD ["./tiara-api"]