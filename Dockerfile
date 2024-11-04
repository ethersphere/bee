FROM golang:1.23 AS build

WORKDIR /src
# enable modules caching in separate layer
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

ARG REACHABILITY_OVERRIDE_PUBLIC=false

RUN make binary REACHABILITY_OVERRIDE_PUBLIC=${REACHABILITY_OVERRIDE_PUBLIC}

FROM debian:12.7-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*; \
    groupadd -r bee --gid 999; \
    useradd -r -g bee --uid 999 --no-log-init -m bee;

# make sure mounted volumes have correct permissions
RUN mkdir -p /home/bee/.bee && chown 999:999 /home/bee/.bee

COPY --from=build /src/dist/bee /usr/local/bin/bee

EXPOSE 1633 1634
USER bee
WORKDIR /home/bee
VOLUME /home/bee/.bee

ENTRYPOINT ["bee"]
