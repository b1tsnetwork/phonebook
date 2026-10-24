# syntax=docker/dockerfile:1
# Mehrplattform-Build (amd64/arm64) ohne Emulation: Go baut auf der Build-Plattform quer.
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum* ./
COPY . .
# go mod tidy ergänzt fehlende go.sum-Einträge (ist go.sum vollständig eingecheckt, ändert sich nichts)
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod tidy && go mod verify
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/telefonbuch . && \
    mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="Telefonbuch" \
      org.opencontainers.image.description="Internes Telefonbuch mit Schlagworten, Pixel-Avataren, LDAPS und MySQL/JSON"
COPY --from=build /out/telefonbuch /telefonbuch
# Arbeitsverzeichnis für den JSON-Betrieb (als Volume einhängen)
COPY --from=build --chown=nonroot:nonroot /out/data /data
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/telefonbuch", "healthcheck"]
ENTRYPOINT ["/telefonbuch"]
CMD ["-config", "/etc/telefonbuch/config.json"]
