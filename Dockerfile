# Builds solas-apiserver.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal/ internal/
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/solas-apiserver ./cmd/solas-apiserver

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/solas-apiserver /solas-apiserver
USER 65532:65532
ENTRYPOINT ["/solas-apiserver"]
