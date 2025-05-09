FROM golang:alpine AS build-env

# Set up dependencies
ENV PACKAGES git build-base linux-headers

# Set working directory for the build
WORKDIR /go/src/github.com/RWAs-labs/ethermint

# hadolint ignore=DL3018
RUN apk add --no-cache $PACKAGES

# Add source files
COPY . .

# Make the binary
RUN make build

# Final image
FROM alpine:3.20.1

WORKDIR /

# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates jq

# Copy over binaries from the build-env
COPY --from=build-env /go/src/github.com/RWAs-labs/ethermint/build/ethermintd /usr/bin/ethermintd

# Run ethermintd by default
CMD ["ethermintd"]
