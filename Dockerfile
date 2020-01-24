FROM golang:1.13 AS build

ARG COMMIT=""

WORKDIR /src
COPY . ./

RUN make binary COMMIT=$COMMIT

FROM debian:10.2-slim

ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update && apt-get install -y \
        ca-certificates && \
    apt-get clean && rm -rf /var/lib/apt/lists/* && \
    groupadd -r drone && \
    useradd --no-log-init -r -g drone drone

COPY --from=build /src/dist/bee /usr/local/bin/bee

USER drone

ENTRYPOINT ["bee"]
