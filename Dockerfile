# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /gopherledger ./cmd/server

# Run stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /gopherledger .
COPY --from=builder /app/config.example.yaml ./config.yaml

EXPOSE 8080

ENTRYPOINT ["./gopherledger"]