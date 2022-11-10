FROM golang:1.19-alpine AS build

WORKDIR /go/src/filter-proxy

COPY go.mod go.sum ./
COPY *.go ./

RUN addgroup -S -g 1001 filter-proxy && adduser -S -D -H -G filter-proxy -u 1001 filter-proxy
RUN CGO_ENABLED=0 \
    go build -v \
      -o /go/bin/filter-proxy \
      *.go

FROM alpine:3.16.0

COPY --from=build /go/bin/filter-proxy /usr/local/bin/filter-proxy
COPY --from=build /etc/passwd /etc/group /etc/

USER filter-proxy
ENTRYPOINT ["/usr/local/bin/filter-proxy"]
