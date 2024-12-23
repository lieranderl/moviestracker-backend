FROM golang:1.23-bullseye AS build-env
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o downloadimdbdata cmd/main.go

FROM golang:1.23-bullseye
WORKDIR /app
RUN mkdir /app/store
COPY --from=build-env /app/downloadimdbdata /app/downloadimdbdata
CMD ["/app/downloadimdbdata"]
