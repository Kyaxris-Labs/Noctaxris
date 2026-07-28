# Digests pinned for supply-chain integrity (refresh when intentionally upgrading bases).
FROM golang:1.26.5-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651 AS build
WORKDIR /src
ARG VERSION=1.2.0
ARG GIT_SHA=unknown
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
  -ldflags="-s -w -X github.com/Kyaxris-Labs/Noctaxris/internal/version.Version=${VERSION}" \
  -o /noctaxris ./cmd/noctaxris
RUN mkdir -p /var/lib/noctaxris && chown 65532:65532 /var/lib/noctaxris

FROM gcr.io/distroless/static-debian12:nonroot@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35
ARG VERSION=1.2.0
ARG GIT_SHA=unknown
LABEL org.opencontainers.image.title="Noctaxris" \
  org.opencontainers.image.description="AWS-shaped local security lab emulator" \
  org.opencontainers.image.source="https://github.com/Kyaxris-Labs/Noctaxris" \
  org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${GIT_SHA}"
USER nonroot:nonroot
COPY --from=build /noctaxris /noctaxris
COPY --from=build --chown=65532:65532 /var/lib/noctaxris /var/lib/noctaxris
EXPOSE 4566
ENTRYPOINT ["/noctaxris"]
