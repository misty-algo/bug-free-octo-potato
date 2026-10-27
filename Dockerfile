FROM golang:1.25-alpine AS builder

# 国内网络环境：proxy.golang.org 与 sum.golang.org 均不可达，
# 必须显式指定国内 Go 模块代理，否则 go mod download 会 connection refused。
ENV GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct \
    GOSUMDB=off

WORKDIR /src
COPY go.mod go.sum ./
# 国内网络下批量下载偶发 unexpected EOF（连接被重置），
# 降并发 + 失败重试，避免整个镜像构建因单次网络抖动失败。
ENV GOMAXPROCS=2
RUN set -e; \
    ok=0; \
    for i in 1 2 3 4 5 6; do \
      if go mod download; then ok=1; break; fi; \
      echo ">>> go mod download 第 $i 次失败，等待后重试..."; \
      sleep 5; \
    done; \
    [ "$ok" = "1" ] || (echo ">>> go mod download 重试 6 次仍失败" && exit 1)
COPY . .
# 每个服务单独 build，一个编译失败不影响其他服务的定位。
RUN go build -o /out/gateway ./cmd/gateway && \
    go build -o /out/user ./cmd/user && \
    go build -o /out/note ./cmd/note && \
    go build -o /out/order ./cmd/order && \
    go build -o /out/passenger ./cmd/passenger && \
    go build -o /out/passengerdb ./cmd/passengerdb

FROM alpine:3.22

WORKDIR /app
COPY --from=builder /out/ /app/
COPY configs/ /app/configs/
# 日志与数据落点在容器内，由 compose 挂卷持久化。
RUN mkdir -p /app/logs /app/data
EXPOSE 8080 9001 9002 9100 9005
CMD ["/app/gateway"]
