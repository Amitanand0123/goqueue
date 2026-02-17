# CSS build stage
FROM node:20-alpine AS css-builder

WORKDIR /app

COPY package.json tailwind.config.js ./
COPY web/static/css/input.css ./web/static/css/input.css
COPY web/templates ./web/templates

RUN npm install
RUN npx tailwindcss -i ./web/static/css/input.css -o ./web/static/css/tailwind.css --minify

# Go build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=css-builder /app/web/static/css/tailwind.css ./web/static/css/tailwind.css

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
