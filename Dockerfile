# syntax=docker/dockerfile:1.18

ARG GO_IMAGE=golang:1.26.8-bookworm@sha256:9fdc884aacc3bec89b20ffc69f4bb369c78210e3e4f600387b5128b12c199f81
FROM ${GO_IMAGE} AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal

ARG APP
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN test -n "${APP}" && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -mod=readonly -trimpath -ldflags="-s -w -buildid=" -o /out/app ./cmd/${APP}

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=build --chown=nonroot:nonroot /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
