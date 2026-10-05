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
COPY go.mod ./
COPY cmd/ cmd/
COPY internal/ internal/
COPY examples/ examples/
CMD ["go", "run", "./cmd/glypha"]

FROM server-dev AS server-build
RUN go test ./... && CGO_ENABLED=0 go build -trimpath -o /out/glypha ./cmd/glypha

FROM scratch AS runtime
COPY --from=server-build /out/glypha /glypha
COPY --from=renderer-build /app/renderer/dist /web
USER 65532:65532
ENV GLYPHA_ADDR=:8080 GLYPHA_WEB_DIR=/web
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s CMD ["/glypha", "healthcheck"]
ENTRYPOINT ["/glypha"]
