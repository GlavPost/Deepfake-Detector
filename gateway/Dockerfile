FROM golang:1.26.2-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o gateway main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/gateway .
COPY ./static ./static

RUN mkdir /shared_data
EXPOSE 8080
CMD ["./gateway"]