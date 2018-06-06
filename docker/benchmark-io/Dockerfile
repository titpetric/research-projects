# Build container
FROM golang:1.10-alpine as backend-build

COPY . /go/src/dco
WORKDIR /go/src/dco

RUN apk --no-cache add gcc musl-dev git
RUN go build -ldflags '-linkmode external -extldflags -static -w' -o dco cmd/benchmark/*.go

# Final image
FROM alpine:3.6

COPY --from=backend-build /go/src/dco/dco /dco/
COPY index.html /dco/

WORKDIR /data/

EXPOSE 8080
ENTRYPOINT ["/dco/dco"]