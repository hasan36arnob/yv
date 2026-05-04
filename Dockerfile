# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go build -o todopro main.go

# Runtime stage
FROM alpine:3.18
RUN adduser -D appuser
WORKDIR /app
COPY --from=builder /app/todopro ./todopro
ENV DATA_FILE=/data/tasks.json
VOLUME ["/data"]
EXPOSE 5000
USER appuser
CMD ["./todopro"]
