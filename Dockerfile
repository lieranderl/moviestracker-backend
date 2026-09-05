# syntax=docker/dockerfile:1.18

ARG GO_IMAGE=golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b
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
