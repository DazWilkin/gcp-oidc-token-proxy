# OIDC Token Proxy for [GCP](https://cloud.google.com)

[![build-container](https://github.com/DazWilkin/gcp-oidc-token-proxy/actions/workflows/build-container.yml/badge.svg)](https://github.com/DazWilkin/gcp-oidc-token-proxy/actions/workflows/build-container.yml)

A way to configure Prometheus to scrape services deployed to [Google Cloud Platform (GCP)](https://cloud.google.com) that require authentication (using Google-minted OpenID Connect ID Tokens).

Prometheus supports only TLS and [OAuth2](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#oauth2) for authenticating scrape targets. Unfortunately, the [OAuth2](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#oauth2) configuration is insufficiently flexible to permit using a Google Service Account as an identity and to mine ID Tokens with the Service Account. This 'sidecar' (thanks [Levi Harrison](https://github.com/leviharrison) for the suggestion in [this](https://stackoverflow.com/a/69419467/609290) comment thread) performs as a proxy, configured to run as a Google Service Account, that mints ID Tokens that can be used by Prometheus' OAuth2 config.

In what follows, I am using a Google [Cloud Run] service by way of example but the principle *should* extend to any service that supports Google ID tokens.

> **CAVEAT** Because of the way that the sidecar intercepts Prometheus' OAuth processing and because Google require's that ID tokens contain an audience value that reflects the Cloud Run service URL and because these URLs are service-specific, unfortunately, one limitation of this solution is that **each** Cloud Run service requires its own Prometheus `job_name`.

+ Create [Service Account](#service-account)
+ [Run](#run) this proxy
+ Configure [Prometheus](#prometheus) to point at it
+ [Scrape](#scrape) e.g. Cloud Run services

## Service Account

```bash

ACCOUNT="oidc-token-proxy"
EMAIL="${ACCOUNT}@${PROJECT}.iam.gserviceaccount.com"

gcloud iam service-accounts create ${ACCOUNT} \
--display-name="OIDC Token Proxy" \
--description="Used by Prometheus to authenticate GCP services" \
--project=${PROJECT}

gcloud iam service-accounts keys create ${PWD}/key.json \
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

TBD

### Docker Compose

```YAML
gcp-oidc-token-proxy:
  restart: always
  depends_on:
  - prometheus
  image: ghcr.io/dazwilkin/gcp-oidc-token-proxy:44f5bf9b4f7bb292691263c149f1defe2cc89efc
  container_name: gcp-oidc-token-proxy
  command:
    # Replace the target_url value with the URL of e.g. Cloud Run service
    - --target_url=https://some-service-xxxxxxxxxx-xx.a.run.app
    # Use which port value you wish
    - --port=7777
  environment:
    GOOGLE_APPLICATION_CREDENTIALS: /secrets/key.json
  volumes:
  - ${PWD}/key.json:/secrets/key.json
  expose:
  - "7777"
  ports:
  # Exposed on host only to facilitate testing
  # E.g. ttp://localhost:7777/metrics
  - 7777:7777
```

### Docker

```bash
# Replace ENDPOINT value with the URL of e.g. Cloud Run service
ENDPOINT="https://some-service-xxxxxxxxxx-xx.a.run.app"
PORT="7777"

# The Service Account is mounted so that it is accessible to the container
docker run \
--interactive --tty --rm \
--publish=7777:7777 \
--volume=${PWD}/key.json:/secrets/key.json \
--env=GOOGLE_APPLICATION_CREDENTIALS=/secret/key.json \
ghcr.io/dazwilkin/gcp-oidc-token-proxy:44f5bf9b4f7bb292691263c149f1defe2cc89efc \
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
- job_name: "gcp-oidc-token-proxy"
  static_configs:
  - targets:
    - "gcp-oidc-token-proxy:7777"
```

## Scrape

![Prometheus: Targets](/images/prometheus.targets.png)
