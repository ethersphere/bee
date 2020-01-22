FROM golang AS builder
ADD . /src
WORKDIR /src
RUN make binary

FROM debian
RUN apt update && apt install -y \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /src/dist/bee /usr/local/bin/bee
RUN chmod +x /usr/local/bin/bee
ENTRYPOINT ["bee"]
