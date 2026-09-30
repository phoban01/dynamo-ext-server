# Builds the solas image, and the demo and pivot images. Pick one with
# --target.
# The build stage runs on the build platform and cross-compiles, so a
# multi-platform build needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal/ internal/
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/solas ./cmd/solas-demo ./cmd/solas-pivot

FROM gcr.io/distroless/static-debian12:nonroot AS solas
COPY --from=build /out/solas /solas
USER 65532:65532
ENTRYPOINT ["/solas"]

FROM gcr.io/distroless/static-debian12:nonroot AS demo
COPY --from=build /out/solas-demo /solas-demo
USER 65532:65532
ENTRYPOINT ["/solas-demo"]

FROM gcr.io/distroless/static-debian12:nonroot AS pivot
COPY --from=build /out/solas-pivot /solas-pivot
USER 65532:65532
ENTRYPOINT ["/solas-pivot"]
