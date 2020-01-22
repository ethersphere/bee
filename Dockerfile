FROM golang:1.13 AS build

WORKDIR /src
COPY . ./

RUN make binary

FROM debian:10.2

ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update && apt-get install -y \
        ca-certificates && \
    apt-get clean && rm -rf /var/lib/apt/lists/* && \
    groupadd -r drone && \
    useradd --no-log-init -r -g drone drone

USER drone

COPY --from=build /src/dist/bee /usr/local/bin/bee

ENTRYPOINT ["bee"]
