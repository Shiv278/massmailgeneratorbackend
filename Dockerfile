# Dockerfile for Golang backend
FROM golang:1.20 AS builder
WORKDIR /app
COPY . .
RUN go mod tidy
RUN go build -o app .

FROM debian:bullseye-slim
WORKDIR /app
COPY --from=builder /app/app .
CMD ["./app"]