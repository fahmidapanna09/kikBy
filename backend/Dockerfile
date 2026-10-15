FROM golang:1.22-alpine AS build
ARG CMD_PATH=./cmd/api
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app ${CMD_PATH}

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /out/app /usr/local/bin/app
EXPOSE 8080
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/app"]
