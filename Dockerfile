ARG GOLANG_VERSION="1.17"
ARG PROJECT="prometheus-oauth-proxy"

ARG COMMIT
ARG VERSION

ARG TOKEN

FROM golang:${GOLANG_VERSION} as build

ARG PROJECT

ARG COMMIT
ARG VERSION

ARG TOKEN

WORKDIR /${PROJECT}

COPY go.mod go.mod
COPY go.sum go.sum

# Configure git to use the GitHub PAT
RUN git config \
    --global url."https://${TOKEN}@github.com".insteadOf "https://github.com"

# Define GOPRIVATE for this environment to circumvent Go Module proxy
ENV GOPRIVATE="github.com/brabantcourt"

RUN go mod download

COPY cmd/proxy cmd/proxy

RUN BUILD_TIME=$(date +%s) && \
    CGO_ENABLED=0 GOOS=linux go build \
    -a \
    -installsuffix cgo \
    -ldflags "-X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${COMMIT}' -X 'main.OSVersion=${VERSION}'" \
    -o /bin/proxy \
    ./cmd/proxy

RUN useradd --uid=10001 scratchuser


FROM scratch

LABEL org.opencontainers.image.source https://github.com/DazWilkin/prometheus-oauth-proxy

COPY --from=build /bin/proxy /
COPY --from=build /etc/passwd /etc/passwd

USER scratchuser

ENTRYPOINT ["/proxy"]
CMD ["--port=7777"]
