FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/gateway ./cmd/gateway && \
    go build -o /out/user ./cmd/user

FROM alpine:3.22

WORKDIR /app
COPY --from=builder /out/ /app/
COPY configs/ /app/configs/
EXPOSE 8080 9001
CMD ["/app/gateway"]
