ARG BASE_IMAGE=scratch

FROM golang:1.21-alpine3.19
WORKDIR /app
COPY . .
WORKDIR /app
ENV GOSUMDB=off
ENV GOWORK=off
RUN apk add --no-cache gcc musl-dev linux-headers git
RUN now=$(date +'%Y-%m-%d_%T') && \
    go build -ldflags "-X main.sha1ver=`git rev-parse HEAD` -X main.buildTime=$now" -mod=vendor -o aws cmd/aws/main.go 

# cannot use scratch becaue of the ca-certificates & hosntame -i command used by the service
FROM alpine:3.19
#RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
#    && apt-get clean
RUN apk --no-cache add ca-certificates \
    && rm -rf /var/cache/apk/*
    
COPY --from=0 /app/aws /
CMD ["/aws"]
