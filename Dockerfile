ARG GOLANG_VERSION="1.17"
ARG PROJECT="prometheus-oauth-proxy"

ARG COMMIT
ARG VERSION

FROM golang:${GOLANG_VERSION} as build

ARG PROJECT

ARG COMMIT
ARG VERSION

WORKDIR /${PROJECT}

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY cmd/proxy cmd/proxy

RUN BUILD_TIME=$(date +%s) && \
    CGO_ENABLED=0 GOOS=linux go build \
    -a \
    -installsuffix cgo \
    -ldflags "-X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${COMMIT}' -X 'main.OSVersion=${VERSION}'" \
    -o /bin/proxy \
    ./cmd/proxy

# RUN useradd --uid=10001 scratchuser


FROM scratch

LABEL org.opencontainers.image.source https://github.com/DazWilkin/gcp-oidc-token-proxy

COPY --from=build /bin/proxy /
COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# USER scratchuser

ENTRYPOINT ["/proxy"]
CMD ["--port=7777"]
