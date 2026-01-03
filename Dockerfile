ARG DOZZLE_VERSION=8.14.12
ARG GO_VERSION=1.22.6

FROM golang:${GO_VERSION}-alpine AS configbuild

WORKDIR /src/config-proxy
COPY config-proxy /src/config-proxy
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/config-proxy .

FROM amir20/dozzle:v${DOZZLE_VERSION} AS dozzle

FROM cloudron/base:4.2.0@sha256:46da2fffb36353ef714f97ae8e962bd2c212ca091108d768ba473078319a47f4

RUN apt-get update \
    && apt-get install -y --no-install-recommends openssh-client ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=dozzle /dozzle /usr/local/bin/dozzle
COPY --from=configbuild /out/config-proxy /usr/local/bin/config-proxy
COPY start.sh /app/code/start.sh

RUN rm -rf /data && ln -s /app/data /data && chmod +x /app/code/start.sh

EXPOSE 8080

CMD ["/app/code/start.sh"]
