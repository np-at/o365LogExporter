FROM alpine:3.15 as base


FROM golang:1.17-alpine as build
WORKDIR /go/src/app
COPY src/ .
RUN apk update && apk add --no-cache git

RUN go get -d -v ./
RUN go build -v

FROM base as run
RUN apk add --no-cache curl
ENV APP_HISTORY_FILE="/app/history"
ENV APP_RUN_INTERVAL="1h"
RUN mkdir /app
RUN adduser -D go_user
WORKDIR /app
COPY --from=build /go/src/app/o365logexporter .
RUN chown -R go_user /app
USER go_user
HEALTHCHECK --interval=5m --timeout=4s  CMD curl --fail http://localhost:8090/live || exit 1

ENTRYPOINT ["/app/o365logexporter","--daemonize"]