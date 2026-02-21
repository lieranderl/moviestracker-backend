FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/getimdb ./cmd/main.go

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/getimdb /app/getimdb
USER nonroot:nonroot
ENTRYPOINT ["/app/getimdb"]
