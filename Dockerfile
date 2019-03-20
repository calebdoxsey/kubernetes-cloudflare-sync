FROM golang:1.12-alpine as build
ENV GO111MODULE=on

RUN apk add --update --no-cache build-base git

# Create a "nobody" non-root user for the next image by crafting an /etc/passwd
# file that the next image can copy in. This is necessary since the next image
# is based on scratch, which doesn't have adduser, cat, echo, or even sh.
RUN echo "nobody:x:65534:65534:Nobody:/:" > /etc_passwd

WORKDIR /src



COPY go.mod ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 go build -o /bin/kubernetes-cloudflare-sync .

FROM scratch

# Copy the certs from the builder stage
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the /etc_passwd file we created in the builder stage into /etc/passwd in
# the target stage. This creates a new non-root user as a security best
# practice.
COPY --from=0 /etc_passwd /etc/passwd

COPY --from=build /bin/kubernetes-cloudflare-sync /bin/kubernetes-cloudflare-sync

USER nobody

ENTRYPOINT ["/bin/kubernetes-cloudflare-sync"]
