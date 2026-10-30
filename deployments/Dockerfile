FROM golang:1.25-alpine AS builder

WORKDIR /app

ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -o agent ./cmd/agent
RUN CGO_ENABLED=0 GOOS=linux go build -o conversation-cleanup ./cmd/conversation_cleanup

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/migrate .
COPY --from=builder /app/agent .
COPY --from=builder /app/conversation-cleanup .
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080 8090

CMD ["./server"]
