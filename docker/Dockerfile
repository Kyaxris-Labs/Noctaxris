FROM golang:1.26.5-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /noctaxris ./cmd/noctaxris
RUN mkdir -p /var/lib/noctaxris && chown 65532:65532 /var/lib/noctaxris

FROM gcr.io/distroless/static-debian12:nonroot
USER nonroot:nonroot
COPY --from=build /noctaxris /noctaxris
COPY --from=build --chown=65532:65532 /var/lib/noctaxris /var/lib/noctaxris
EXPOSE 4566
ENTRYPOINT ["/noctaxris"]
