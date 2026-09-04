# A binary that reads a file and makes two GET requests has no business owning a shell, a package
# manager or root: the final image is `scratch` plus the static binary and the CA bundle it needs
# to fetch a robots.txt over HTTPS.
FROM golang:1.24-alpine AS build
ARG VERSION=docker
WORKDIR /src
COPY . .
# CGO off is what makes the binary runnable on an empty image; -trimpath keeps build paths out of it.
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /robotsmith .

FROM scratch
# Without the CA bundle every https fetch fails with "certificate signed by unknown authority".
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /robotsmith /robotsmith
# 65534 is `nobody`: the container writes nothing, so it needs no identity of its own.
USER 65534:65534
ENTRYPOINT ["/robotsmith"]
