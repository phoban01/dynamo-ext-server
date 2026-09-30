# Builds solas-apiserver and solas-controller. Pick one with --target.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal/ internal/
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/solas-apiserver ./cmd/solas-controller ./cmd/solas-demo

FROM gcr.io/distroless/static-debian12:nonroot AS controller
COPY --from=build /out/solas-controller /solas-controller
USER 65532:65532
ENTRYPOINT ["/solas-controller"]

FROM gcr.io/distroless/static-debian12:nonroot AS apiserver
COPY --from=build /out/solas-apiserver /solas-apiserver
USER 65532:65532
ENTRYPOINT ["/solas-apiserver"]

FROM gcr.io/distroless/static-debian12:nonroot AS demo
COPY --from=build /out/solas-demo /solas-demo
USER 65532:65532
ENTRYPOINT ["/solas-demo"]
