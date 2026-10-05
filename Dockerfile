# syntax=docker/dockerfile:1
FROM node:24-bookworm-slim AS renderer-dev
WORKDIR /app/renderer
COPY renderer/package*.json ./
RUN npm ci
COPY renderer/ ./
CMD ["npm", "run", "dev"]

FROM renderer-dev AS renderer-build
RUN npm run build

FROM golang:1.27.1-bookworm AS server-dev
WORKDIR /app
ENV GLYPHA_ADDR=:8080
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY examples/ examples/
COPY renderer/public/fonts/ renderer/public/fonts/
CMD ["go", "run", "./cmd/glypha"]

FROM server-dev AS server-build
RUN go test ./... && CGO_ENABLED=0 go build -trimpath -o /out/glypha ./cmd/glypha
RUN mkdir /data && chown 65532:65532 /data

FROM scratch AS runtime
COPY LICENSE /LICENSE
COPY --from=server-build /out/glypha /glypha
COPY --from=renderer-build /app/renderer/dist /web
COPY --from=server-build --chown=65532:65532 /data /data
USER 65532:65532
ENV GLYPHA_ADDR=:8080 GLYPHA_WEB_DIR=/web GLYPHA_DB_PATH=/data/glypha.db GLYPHA_FONT_PATH=/web/fonts/noto-sans-jp/NotoSansJP.ttf
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s CMD ["/glypha", "healthcheck"]
ENTRYPOINT ["/glypha"]
