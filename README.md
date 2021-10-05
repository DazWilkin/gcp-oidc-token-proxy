# OIDC Token Proxy for [GCP](https://cloud.google.com)

[![build-container](https://github.com/DazWilkin/gcp-oidc-token-proxy/actions/workflows/build-container.yml/badge.svg)](https://github.com/DazWilkin/gcp-oidc-token-proxy/actions/workflows/build-container.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dazwilkin/gcp-oidc-token-proxy)](https://goreportcard.com/report/github.com/dazwilkin/gcp-oidc-token-proxy)

A way to configure Prometheus to scrape services deployed to [Google Cloud Platform (GCP)](https://cloud.google.com) that require authentication (using Google-minted OpenID Connect ID Tokens).

Prometheus supports only TLS and [OAuth2](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#oauth2) for authenticating scrape targets. Unfortunately, the [OAuth2](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#oauth2) configuration is insufficiently flexible to permit using a Google Service Account as an identity and to mine ID Tokens with the Service Account. This 'sidecar' (thanks [Levi Harrison](https://github.com/leviharrison) for the suggestion in [this](https://stackoverflow.com/a/69419467/609290) comment thread) performs as a proxy, configured to run as a Google Service Account, that mints ID Tokens that can be used by Prometheus' OAuth2 config.

Thanks also to [Salmaan](https://github.com/salrashid123) for guidance navigating Google's seeming myriad of OAuth2 and ID token related libraries and for the short-circuit in using ID tokens as the Bearer value.

In what follows, I am using a Google [Cloud Run] service by way of example but the principle *should* extend to any service that supports Google ID tokens.

> **CAVEAT** Because of the way that the sidecar intercepts Prometheus' OAuth processing and because Google require's that ID tokens contain an audience value that reflects the Cloud Run service URL and because these URLs are service-specific, unfortunately, one limitation of this solution is that **each** Cloud Run service requires its own Prometheus `job_name`.

+ Deploy a [Cloud Run](#cloud-run) service to be scraped
+ Create [Service Account](#service-account)
+ [Run](#run) this proxy
+ Configure [Prometheus](#prometheus) to point at it
+ [Scrape](#scrape) e.g. Cloud Run services

## Cloud Run service

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

> **NOTE** The protocol (scheme) (usually `https`) and the `//` are removed from the `ENDPOINT` value as this is not expected by Prometheus targets.

## Service Account

```bash
ACCOUNT="oidc-token-proxy"
EMAIL="${ACCOUNT}@${PROJECT}.iam.gserviceaccount.com"

gcloud iam service-accounts create ${ACCOUNT} \
--display-name="OIDC Token Proxy" \
--description="Used by Prometheus to authenticate GCP services" \
--project=${PROJECT}

gcloud iam service-accounts keys create ${PWD}/${ACCOUNT}.json \
--iam-account=${EMAIL} \
--project=${PROJECT}

# The Service Account is able to invoke Cloud Run services
# This value should be adjusted for other GCP services
# https://cloud.google.com/run/docs/reference/iam/roles
gcloud projects add-iam-policy-binding ${PROJECT} \
--member=serviceAccount:${EMAIL} \
--role=roles/run.invoker
```

## Run

### Kubernetes

There are 3 steps to deploy the solution to Kubernetes:

+ Create Namespace
+ Create ConfigMap for Prometheus' config
+ Create Secret for Google Service Account
+ Deploy Prometheus w/ `gcp-oidc-token-proxy` sidecar

> **NOTE** `gcp-oidc-token-proxy` need not be deployed as a sidecar.

```bash
NAMESPACE="gcp-oidc-token-proxy"
CONFIG="prometheus-config"
SECRET="${ACCOUNT}"

kubectl create namespace ${NAMESPACE}

# Revise the Cloud Run service URL in prometheus.yml before creating the ConfigMap
# The Deployment runs the the proxy as a sidecar so the two references to it, as a
# target and as the token_url value should be left as localhost
CONFIGMAP=$(mktemp)

sed \
--expression="s|some-service-xxxxxxxxxx-yy.a.run.app|${ENDPOINT}|g" \
${PWD}/prometheus.yml > ${CONFIGMAP}

kubectl create configmap ${CONFIG} \
--from-file=prometheus.yml=${CONFIGMAP} \
--namespace=${NAMESPACE}

kubectl create secret generic ${SECRET} \
--from-file=key.json=${PWD}/${ACCOUNT}.json \
--namespace=${NAMESPACE}

# Deployment
# Revise manually or using sed before creating
DEPLOYMENT=$(mktemp)

sed \
--expression="s|https://some-service-xxxxxxxxxx-yy.a.run.app|https://${ENDPOINT}|g" \
--expression="s|name: CONFIG|name: ${CONFIG}|g" \
--expression="s|secretName: SECRET|secretName: ${SECRET}|g" \
${PWD}/kubernetes/deployment.yml > ${DEPLOYMENT}

kubectl create --filename=${DEPLOYMENT} \
--namespace=${NAMESPACE}

# Don't do this in production
kubectl port-forward deployment/prometheus \
--namespace=${NAMESPACE} \
9090:9090
```

Once the deployment completes, you should be able to browse Prometheus' UI on `localhost:9090`

### Docker Compose

Ensure `prometheus.yml` reflects the correct Cloud Run service URL.

> **NOTE** `localhost` values will work when using Docker Compose but it is better to use the services' internal DNS name. In this case, `localhost:9090` becomes `prometheus:9090` and the two occurrences of `localhost:7777` should be replaced by `gcp-oidc-token-proxy:7777`:

```bash
sed \
--in-place \
--expression="s|localhost:9090|prometheus:9090|g" \
--expression="s|localhost:7777|gcp-oidc-token-proxy:7777|g" \
--expression="s|some-service-xxxxxxxxxx-yy.a.run.app|${ENDPOINT}|g" \
${PWD}/prometheus.yml
```

And:

```YAML
gcp-oidc-token-proxy:
  restart: always
  depends_on:
  - prometheus
  image: ghcr.io/dazwilkin/gcp-oidc-token-proxy:22cd681b35583cb4efdbf8e2d3fbbd9ed641908f
  container_name: gcp-oidc-token-proxy
  command:
    # Replace the target_url value with the URL of e.g. Cloud Run service
    - --target_url=https://some-service-xxxxxxxxxx-yy.a.run.app
    # Use which port value you wish
    - --port=7777
  environment:
    GOOGLE_APPLICATION_CREDENTIALS: /secrets/key.json
  volumes:
  # Replace ACCOUNT with the correct values
  - ${PWD}/ACCOUNT/key.json:/secrets/key.json
  expose:
  - "7777"
  ports:
  # Exposed on host only to facilitate testing
  # E.g. ttp://localhost:7777/metrics
  - 7777:7777
```

Then e.g. `docker-compose up`

### Docker

```bash
# Replace ENDPOINT value with the URL of e.g. Cloud Run service
ENDPOINT="https://some-service-xxxxxxxxxx-yy.a.run.app"
PORT="7777"

# The Service Account is mounted so that it is accessible to the container
docker run \
--interactive --tty --rm \
--publish=7777:7777 \
--volume=${PWD}/key.json:/secrets/${ACCOUNT}.json \
--env=GOOGLE_APPLICATION_CREDENTIALS=/secret/key.json \
ghcr.io/dazwilkin/gcp-oidc-token-proxy:22cd681b35583cb4efdbf8e2d3fbbd9ed641908f \
  --target_url=${ENDPOINT} \
  --port=${PORT}
```

## Prometheus

The Prometheus configuration file (`prometheus.yml`) needs to include an `[OAuth]` section that points to the proxy.

```YAML
# E.g. Cloud Run service
- job_name: "cloudrun-service"
  scheme: https
  oauth2:
    client_id: "anything"
    client_secret: "anything"
    scopes:
    - "https://www.googleapis.com/auth/cloud-platform"
    token_url: "http://gcp-oidc-token-proxy:7777/"
  static_configs:
  - targets:
    # Port 443 is not strictly necessary here as the scheme is HTTPS and 443 is the default port
    - "some-service-xxxxxxxxxx-yy.a.run.app:443"
```

The proxy exports metrics too and these can be included:

```
# GCP OAuth Token Proxy
- job_name: "gcp-oidc-token-proxy"
  static_configs:
  - targets:
    - "gcp-oidc-token-proxy:7777"
```

## Scrape

![Prometheus: Targets](/images/prometheus.targets.png)
