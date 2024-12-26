FROM golang:1.23-bullseye AS build-env
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -o gettorrents cmd/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=build-env /app/gettorrents /app/gettorrents
CMD ["/app/gettorrents"]
