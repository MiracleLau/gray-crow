# 阶段 1：构建前端
FROM oven/bun:1-alpine AS web
# vue-tsc 需要真正的 node：bun 运行时跑它不会加载 vue 语言插件，会误报一堆 "Cannot find module '*.vue'"
RUN apk add --no-cache nodejs
WORKDIR /app/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

# 阶段 2：编译后端（把前端产物一起 embed）
FROM golang:1.26-alpine AS api
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/embed.go ./web/embed.go
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /app/graycrow ./cmd/graycrow

# 阶段 3：运行
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=api /app/graycrow /app/graycrow
ENV PORT=8080 \
    DB_DRIVER=sqlite \
    DB_DSN=/data/graycrow.db
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/graycrow"]
