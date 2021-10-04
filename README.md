# Prometheus OAuth2 Proxy for GCP

## Setup

```bash
BILLING=$(gcloud alpha billing accounts list --format="value(name)")
PROJECT="dazwilkin-$(date +%y%m%d)-oauthproxy"
REGION="us-west2"
REPOSITORY="repo"
API_KEY="..."

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
GHCR="ghcr.io/dazwilkin" # dazwilkin for prometheus-oauth-proxy
IMAGE="prometheus-oauth-proxy"

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

### Service Account

```bash
ACCOUNT="cloudrun"
EMAIL="${ACCOUNT}@${PROJECT}.iam.gserviceaccount.com"

gcloud iam service-accounts create ${ACCOUNT} \
--project=${PROJECT}

gcloud iam service-accounts keys create ${PWD}/client_json.json \
--iam-account=${EMAIL} \
--project=${PROJECT}

gcloud projects add-iam-policy-binding ${PROJECT} \
--member=serviceAccount:${EMAIL} \
--role=roles/run.invoker
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
