FROM golang:1.21-alpine3.19
WORKDIR /app
COPY . .
WORKDIR /app
ENV GOSUMDB=off
ENV GOWORK=off 
RUN apk add --no-cache gcc musl-dev linux-headers git
RUN now=$(date +'%Y-%m-%d_%T') && \ 
    go build -ldflags "-X main.version=2.0.0  -X main.sha1ver=`git rev-parse HEAD` -X main.buildTime=$now" -mod=vendor -o ca cmd/ca/main.go 

# Alpine and scartch dont work for this image due to non corss compileable HSM library
FROM alpine:3.19

RUN apk --update add --no-cache gcc musl-dev linux-headers git make cmake libseccomp-dev opensc bash

RUN git clone https://github.com/SUNET/pkcs11-proxy && \
    cd pkcs11-proxy && \
    cmake . && make && make install && make clean

# Clean build artifacts
RUN rm -rf /pkcs11-proxy
RUN apk del -r gcc musl-dev linux-headers git make cmake libseccomp-dev opensc bash 
RUN rm -rf /var/cache/apk/*

COPY --from=0 /app/ca /
CMD ["/ca"]
