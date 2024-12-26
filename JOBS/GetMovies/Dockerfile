FROM golang:1.23-bullseye AS build-env
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -o getmovies cmd/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=build-env /app/getmovies /app/getmovies

ENTRYPOINT ["/app/getmovies"]