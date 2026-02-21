FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/downloadimdbdata ./cmd/main.go

FROM alpine:3.21
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
RUN mkdir -p /app/store && chown -R app:app /app
COPY --from=build /out/downloadimdbdata /app/downloadimdbdata
USER app
ENTRYPOINT ["/app/downloadimdbdata"]
