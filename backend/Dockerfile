# --- Etapa de build ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/cielowave-api .

# --- Etapa final ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/bin/cielowave-api .

EXPOSE 8080

ENTRYPOINT ["/app/cielowave-api"]
