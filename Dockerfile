FROM golang:1.13 AS build

WORKDIR /src
COPY . ./

RUN make binary

FROM alpine:3.11

RUN apk update && \
    apk add --no-cache ca-certificates 

COPY --from=build /src/dist/bee /usr/local/bin/bee

ENTRYPOINT ["bee"]
