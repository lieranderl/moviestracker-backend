FROM golang:1.21-bullseye AS build-env
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o getmovies cmd/main.go 

FROM golang:1.21-bullseye 
WORKDIR /app
COPY --from=build-env /app/getmovies /app/getmovies

ENTRYPOINT ["/app/getmovies"]