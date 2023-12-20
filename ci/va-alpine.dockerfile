ARG BASE_IMAGE=scratch

FROM golang:1.21-alpine3.19
WORKDIR /app
COPY . .
WORKDIR /app
ENV GOSUMDB=off
ENV GOWORK=off

RUN apk add --no-cache gcc musl-dev linux-headers git
RUN now=$(date +'%Y-%m-%d_%T') && \
    go build -ldflags "-X main.sha1ver=`git rev-parse HEAD` -X main.buildTime=$now" -mod=vendor -o va cmd/va/main.go 

FROM alpine:3.19
COPY --from=0 /app/va /
CMD ["/va"]
