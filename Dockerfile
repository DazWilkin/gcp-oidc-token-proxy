ARG GOLANG_VERSION="1.12.0"
ARG PROJECT="gcp-oidc-token-proxy"

ARG COMMIT
ARG VERSION

FROM docker.io/golang:${GOLANG_VERSION} as build

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


FROM gcr.io/distroless/cc

LABEL org.opencontainers.image.source https://github.com/DazWilkin/gcp-oidc-token-proxy

COPY --from=build /bin/proxy /

ENTRYPOINT ["/proxy"]
CMD ["--port=7777"]
