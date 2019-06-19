# Start with a full-fledged golang image, but strip it from the final image.
FROM golang:1.12.6-alpine

RUN apk add build-base

Run which wget

WORKDIR /go/src/github.com/gdotgordon/ipverify

COPY . /go/src/github.com/gdotgordon/ipverify

RUN go build -v

FROM alpine:latest

WORKDIR /root/

# Make a significantly slimmed-down final result.
COPY --from=0 /go/src/github.com/gdotgordon/ipverify .

LABEL maintainer="Gary Gordon <gagordon12@gmail.com>"

ENTRYPOINT ["./ipverify"]
CMD ["--port=8080" "--log=production"]
