ARG BASE_IMAGE=scratch

FROM golang:1.19
WORKDIR /app
ENV GOSUMDB=off
RUN CGO_ENABLED=0 go install -ldflags "-s -w -extldflags '-static'" github.com/go-delve/delve/cmd/dlv@latest

COPY . .
RUN now=$(date +'%Y-%m-%d_%T') && \
    CGO_ENABLED=0 go build -gcflags="all=-N -l" -ldflags "-X main.sha1ver=`git rev-parse HEAD` -X main.buildTime=$now"  -mod=vendor -o /ca cmd/ca/main.go 

CMD [ "/go/bin/dlv", "--listen=:4000", "--headless=true", "--log=true", "--accept-multiclient", "--api-version=2", "exec", "/ca" ]
