FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -mod=readonly -o /out/order ./cmd/order && CGO_ENABLED=0 go build -mod=readonly -o /out/gateway ./cmd/gateway
FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/ /app/
COPY deploy/arrival-config.yaml /app/configs/config.yaml
COPY deploy/arrival-nacos.yaml /app/configs/nacos.yaml
CMD ["/app/gateway"]
