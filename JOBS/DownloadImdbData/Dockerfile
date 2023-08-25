FROM golang:1.21-bullseye AS build-env
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o downloadimdbdata cmd/main.go 

FROM golang:1.21-bullseye 
WORKDIR /app
COPY --from=build-env /app/downloadimdbdata /app/downloadimdbdata
CMD ["/app/downloadimdbdata"]
