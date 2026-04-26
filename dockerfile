FROM golang:1.25-alpine

# Install Air untuk live-reloading
RUN go install github.com/cosmtrek/air@v1.49.0

WORKDIR /app

# Copy dependency dulu biar build lebih cepat (caching)
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Jalankan Air
CMD ["air"]