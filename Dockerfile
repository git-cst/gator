# Stage 1 — build
FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gator .

# Stage 2 — run
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/gator .
EXPOSE 8888
CMD ["./gator"]
