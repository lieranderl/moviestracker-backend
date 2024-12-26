FROM golang:1.23-bullseye AS build-env
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -o downloadimdbdata cmd/main.go

FROM alpine:latest
WORKDIR /app
RUN mkdir /app/store
COPY --from=build-env /app/downloadimdbdata /app/downloadimdbdata
CMD ["/app/downloadimdbdata"]
