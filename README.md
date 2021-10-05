# OIDC Token Proxy for [GCP](https://cloud.google.com)

[![build-container](https://github.com/DazWilkin/gcp-oidc-token-proxy/actions/workflows/build-container.yml/badge.svg)](https://github.com/DazWilkin/gcp-oidc-token-proxy/actions/workflows/build-container.yml)

A way to configure Prometheus to scrape services deployed to [Google Cloud Platform (GCP)](https://cloud.google.com) that require authentication (using Google-minted OpenID Connect ID Tokens)

Prometheus supports only TLS and [OAuth2](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#oauth2) for authenticating scrape targets. Unfortunately, the [OAuth2](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#oauth2) configuration is insufficiently flexible to permit using a Google Service Account as an identity and to mind ID Tokens with the Service Account. This 'sidecar' (thanks Levi Harrison for the suggestion in [this](https://stackoverflow.com/a/69419467/609290) comment thread) performs as a proxy, configured to run as a Google Service Account, that mints ID Tokens that can be used by Prometheus' OAuth2 config.

+ Create [Service Account](#service-account)
+ [Run](#run) this proxy
+ Configure [Prometheus](#prometheus) to point at it
+ [Scrape](#scrape) e.g. Cloud Run services

### Service Account

```bash
ACCOUNT="gcp-oidc-token-proxy"
EMAIL="${ACCOUNT}@${PROJECT}.iam.gserviceaccount.com"

gcloud iam service-accounts create ${ACCOUNT} \
--project=${PROJECT}

gcloud iam service-accounts keys create ${PWD}/key.json \
--iam-account=${EMAIL} \
--project=${PROJECT}

gcloud projects add-iam-policy-binding ${PROJECT} \
--member=serviceAccount:${EMAIL} \
--role=roles/run.invoker
```

## Run

### Kubernetes

TBD

### Docker Compose

```YAML
gcp-oidc-token-proxy:
  restart: always
  depends_on:
  - prometheus
  image: ghcr.io/dazwilkin/gcp-oidc-token-proxy:1e481c723cec01e4f6b71a88a74d62364a33e5fe
  container_name: gcp-oidc-token-proxy
  command:
    - --target_url=https://some-service-xxxxxxxxxx-xx.a.run.app
    - --port=7777
  environment:
    GOOGLE_APPLICATION_CREDENTIALS: /secrets/key.json
  volumes:
  - ${PWD}/key.json:/secrets/key.json
  expose:
  - "7777"
  ports:
  - 7777:7777
```

### Docker

```bash
ENDPOINT="[[e.g. Cloud Run service URL]]"
PORT="7777"

docker run \
--interactive --tty --rm \
--publish=7777:7777 \
--volume=${PWD}/key.json:/secrets/key.json \
--env=GOOGLE_APPLICATION_CREDENTIALS=/secret/key.json \
ghcr.io/dazwilkin/gcp-oidc-token-proxy:1e481c723cec01e4f6b71a88a74d62364a33e5fe \
--target_url=${ENDPOINT} \
--port=${PORT}
```

## Prometheus

The Prometheus configuration file (`prometheus.yml`) needs to include an `[OAuth]` section that points to the proxy:

```YAML
# E.g. Cloud Run service
- job_name: "cloudrun-service"
  scheme: https
  oauth2:
    client_id: "anything"
    client_secret: "anything"
    scopes:
    - "https://www.googleapis.com/auth/cloud-platform"
    token_url: "http://gcp-oauth-token-proxy:7777/"
  static_configs:
    - targets:
        - "some-service-xxxxxxxxxx-yy.a.run.app:443"
```

> **NOTE** The e.g. Cloud Run URL needs to be without the protocol (defined by `scheme`) and the default HTTP port (`443`) is optional in this case.

You can use `gcloud` to grab a Cloud Run service's URL:

```bash
ENDPOINT=$(\
  gcloud run services describe ${NAME} \
  --project=${PROJECT} \
  --platform=managed \
  --region=${REGION} \
  --format="value(status.address.url)") && \
ENDPOINT=${ENDPOINT#https://} && \
echo ${ENDPOINT}
```

The proxy exports metrics too and these can be included:

```
# GCP OAuth Token Proxy
- job_name: "gcp-oauth-token-proxy"
  static_configs:
  - targets:
    - "gcp-oauth-token-proxy:7777"
```

## Scrape

![Prometheus: Targets](/images/prometheus.targets.png)

## Setup

```bash
BILLING=$(gcloud alpha billing accounts list --format="value(name)")
PROJECT="[[YOUR-PROJECT-ID]]"
REGION="[[YOUR-REGION]]"
REPOSITORY="[[YOUR-REPOSITORY]]"

GHCR_TOKEN="..."

gcloud projects create ${PROJECT}
gcloud beta billing projects link ${PROJECT} --billing-account=${BILLING}

for SERVICE in "run" "artifactregistry" "securetoken"
do
  gcloud services enable ${SERVICE}.googleapis.com \
  --project=${PROJECT}
done

gcloud artifacts repositories create ${REPOSITORY} \
--repository-format=docker \
--location=${REGION} \
--project=${PROJECT}

GHCR="ghcr.io/brabantcourt" # brabantcourt for ackal-healthcheck-server
GXR="${REGION}-docker.pkg.dev/${PROJECT}/${REPOSITORY}"

gcloud auth print-access-token \
| docker login \
  --username=oauth2accesstoken \
  --password-stdin https://${REGION}-docker.pkg.dev

TYPE=server

echo ${TOKEN} \
| docker login \
  --username=DazWilkin \
  --password-stdin \
  https://ghcr.io 

docker pull ${GHCR}/ackal-healthcheck-${TYPE}:6f29c437b6b7875edc13cfa48c5ea4dd77e06519
docker tag \
  ${GHCR}/ackal-healthcheck-${TYPE}:6f29c437b6b7875edc13cfa48c5ea4dd77e0 \
  ${GXR}/ackal-healthcheck-${TYPE}:6f29c437b6b7875edc13cfa48c5ea4dd77e06519

docker push ${GXR}/ackal-healthcheck-${TYPE}:6f29c437b6b7875edc13cfa48c5ea4dd77e06519
```

## Build

```bash
GHCR="ghcr.io/dazwilkin"
IMAGE="gcp-oidc-token-proxy"

docker build \
--build-arg=VERSION=$(uname --kernel-release) \
--build-arg=COMMIT=$(git rev-parse HEAD) \
--tag=${GHCR}/${IMAGE}:$(git rev-parse HEAD) \
--file=./Dockerfile \
.

# Update image in Docker Compose
sed \
--in-place \
--expression "s|${GHCR}/${IMAGE}:[0-9a-z]\{40\}|${GHCR}/${IMAGE}:$(git rev-parse HEAD)|g" \
./docker-compose.yml
```

### Healthcheck Server

```bash
NAME="ackal-healthcheck-server"
PORT="50051"

gcloud run deploy ${NAME} \
--args="--failure_rate=0.5","--changes_rate=15s","--endpoint=:${PORT}" \
--max-instances=1 \
--image=${GXR}/ackal-healthcheck-server:6f29c437b6b7875edc13cfa48c5ea4dd77e06519 \
--platform=managed \
--port=${PORT} \
--no-allow-unauthenticated \
--region=${REGION} \
--project=${PROJECT}

ENDPOINT=$(\
  gcloud run services describe ${NAME} \
  --project=${PROJECT} \
  --platform=managed \
  --region=${REGION} \
  --format="value(status.address.url)") && \
ENDPOINT=${ENDPOINT#https://} && \
echo ${ENDPOINT}
```

`${ENDPOINT}` is the value for the service in `prometheus.yml`

```bash
sed --in-place \
--expression "s|-\s"[a-z0-9-]*-[a-z0-9]\{10\}-[a-z]\{2\}.a.run.app:443"|"-\s"${ENDPOINT}:443"|g" \
./prometheus.yml
```



## Test

```bash
# Without Auth should fail (403)
curl \
--silent \
--request GET \
--output /dev/null \
--write-out "%{response_code}" \
https://${ENDPOINT}/metrics

# With Auth should succeed (200)
curl \
--silent \
--request GET \
--header "Authorization: Bearer $(gcloud auth print-identity-token)" \
--output /dev/null \
--write-out "%{response_code}" \
https://${ENDPOINT}/metrics
```

## Run


## Prometheus

### Reload

```bash
curl --request POST \
http://localhost:9090/-/reload
```

## Debugging

Body received:

```JSON
client_id=foo&client_secret=bar&grant_type=client_credentials&scope=https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fcloud-platform
```

### Example log lines

```console
"caller"={"file":"main.go","line":142} "level"=0 "msg"="Request" "Host"="" "Path"="/" "Query"=""
"caller"={"file":"main.go","line":152} "level"=0 "msg"="Body" "Body"="grant_type=client_credentials&scope=https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fcloud-platform"
"caller"={"file":"main.go","line":163} "level"=0 "msg"="Token" "token"={"access_token":"ey...","token_type":"","refresh_token":"","expiry":{}}
```

### Shipping directly to the proxy:

```bash
CLIENT_ID="foo"
CLIENT_SECRET="bar"
GRANT_TYPE="client_credentials"

curl --data "client_id=${CLIENT_ID}&client_secret=${CLIENT_SECRET}&grant_type=${GRANT_TYPE}&scope=https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fcloud-platform" localhost:7777/
```

### Getting directly from proxy and calling Cloud Run:

```bash
TOKEN=$(\
  curl \
  --silent \
  --data "client_id=foo&client_secret=bar&grant_type=client_credentials&scope=https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fcloud-platform" \
  localhost:7777/ \
  | jq -r .access_token)

curl \
--header "Authorization: Bearer ${TOKEN}" \
https://${ENDPOINT}/metrics
```

> **NOTE** The request body is ignored by the sidecar so `foo`, `bar` etc need not be included, just some body

> **NOTE** `localhost:7777` corresponds to `prom-oauth-proxy:7777` from the host

## Program Notes

Cloud Run requires ID Tokens

[`oauth2/google`](https://pkg.go.dev/golang.org/x/oauth2/google) returns Access Tokens ya29...

```golang
ts, err := google.DefaultTokenSource(context.Background(), scopeCloudPlatform)
```

[`idtoken`](https://pkg.go.dev/google.golang.org/api/idtoken) returns ID Tokens
	
```golang
ts, err := idtoken.NewTokenSource(context.Background(), *target_url)
```
