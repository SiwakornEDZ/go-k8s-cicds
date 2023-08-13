FROM golang:1.20 AS build

ENV GO111MODULE=on

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o go-cicds

FROM ubuntu:20.04

RUN apt-get update && apt-get install -y ca-certificates

COPY --from=build /app/go-cicds /app/go-cicds

CMD ["/app/go-cicds"]
