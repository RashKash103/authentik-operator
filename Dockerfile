# Build the manager binary.
# Digest-pinned. A floating tag means the release job can build against
# something other than what was reviewed, and the provenance attestation would
# faithfully attest the substituted result. Dependabot updates digest-pinned
# FROM lines.
FROM golang:1.27.2@sha256:5bc7f572bbaa98885a3a1fd9c0aa76b59e3e14e8628bfc316bbfd0c701e4818c AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Cache dependencies before copying source so a source-only change does not
# re-download the module graph.
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# CGO_ENABLED=0 for a static binary that runs on distroless static.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -a -trimpath -ldflags="-w -s" -o manager cmd/main.go

FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
