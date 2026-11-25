FROM node:22-bookworm AS frontend-builder
WORKDIR /src
COPY frontend/package*.json frontend/
RUN cd frontend && npm ci
COPY shared shared
COPY internal/theme/default.json internal/theme/default.json
COPY internal/database/emoji-defaults.json internal/database/emoji-defaults.json
COPY frontend frontend
RUN cd frontend && npm run build

FROM node:22-bookworm AS admin-builder
WORKDIR /src
COPY admin/package*.json admin/
RUN cd admin && npm ci
COPY shared shared
COPY internal/theme/default.json internal/theme/default.json
COPY internal/database/emoji-defaults.json internal/database/emoji-defaults.json
COPY admin admin
RUN cd admin && npm run build

FROM golang:1.26-bookworm AS backend-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY pkg pkg
RUN CGO_ENABLED=1 go build -trimpath -o /mofufans ./cmd

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates nginx tini && rm -rf /var/lib/apt/lists/*
WORKDIR /work
COPY --from=frontend-builder /src/frontend/out ./frontend
COPY --from=admin-builder /src/admin/out ./admin
COPY --from=backend-builder /mofufans ./mofufans
COPY nginx.conf /etc/nginx/nginx.conf
COPY start.sh ./start.sh
RUN chmod +x start.sh && mkdir -p /work/data /work/config
ENV GIN_MODE=release
EXPOSE 80
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["./start.sh"]
