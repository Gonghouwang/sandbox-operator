ARG GO_VERSION=1.26.0
FROM golang:${GO_VERSION} AS builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/sandbox-operator ./cmd/manager

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/sandbox-operator /sandbox-operator
USER 65532:65532
ENTRYPOINT ["/sandbox-operator"]
