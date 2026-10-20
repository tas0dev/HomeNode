FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package*.json web/build.mjs web/tsconfig.json ./
COPY web/src ./src
COPY license /src/license
RUN npm ci --ignore-scripts && npm run build

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -mod=mod -trimpath -ldflags="-s -w" -o /out/homenode-server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && addgroup -g 10001 homenode && adduser -D -H -u 10001 -G homenode homenode
COPY --from=build /out/homenode-server /usr/local/bin/homenode-server
USER 10001:10001
ENTRYPOINT ["homenode-server"]
CMD ["-config", "/etc/homenode/config.yaml"]
