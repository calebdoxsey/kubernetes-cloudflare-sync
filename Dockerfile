FROM golang:1.11.0-alpine as build
ENV GO111MODULE=on

RUN apk add --update --no-cache build-base git

WORKDIR /src

COPY go.mod .
RUN go mod download

COPY *.go .
RUN go build -o /bin/kubernetes-cloudflare-sync .

FROM alpine:latest

RUN apk add --update --no-cache ca-certificates

COPY --from=build /bin/kubernetes-cloudflare-sync /bin/kubernetes-cloudflare-sync 

ENTRYPOINT ["/bin/kubernetes-cloudflare-sync"]
