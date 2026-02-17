# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o goqueue ./cmd/goqueue

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/goqueue .
COPY --from=builder /app/web ./web

EXPOSE 8080

ENTRYPOINT ["./goqueue"]
CMD ["start"]
