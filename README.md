# OIDC Token Proxy for [GCP](https://cloud.google.com)

[![build-container](https://github.com/DazWilkin/gcp-oidc-token-proxy/actions/workflows/build-container.yml/badge.svg)](https://github.com/DazWilkin/gcp-oidc-token-proxy/actions/workflows/build-container.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dazwilkin/gcp-oidc-token-proxy)](https://goreportcard.com/report/github.com/dazwilkin/gcp-oidc-token-proxy)

A way to configure Prometheus to scrape services deployed to [Google Cloud Platform (GCP)](https://cloud.google.com) that require authentication (using Google-minted OpenID Connect ID Tokens).

> **TL;DR**
>
> ```YAML
> # Cloud Run service
> - job_name: "cloudrun-service"
>   scheme: https
>   oauth2:
>     client_id: "anything"
>     client_secret: "anything"
>     token_url: "http://gcp-oidc-token-proxy:7777"
>     endpoint_params:
>       audience: https://some-service-xxxxxxxxxx-yy.a.run.app
>   static_configs:
>   - targets:
>     - "some-service-xxxxxxxxxx-yy.a.run.app:443"
> ```
>
> **NOTE**
> + `client_id` and `client_secret` must be included but can be any string other than `""`
> + `token_url` is a reference to the endpoint of the GCP OIDC Token Proxy
> + `scopes` is not required (and is mutually exclusive with `audience`)
> + `oauth2.endpoint_params` is used to provide the proxy with an `audience` value
> + For Cloud Run services, the identity token's audience must be the [[TODO]]

## ToC

+ [Background](#background)
+ [Cloud Run service](#cloud-run-service)
+ [Service Account](#service-account)
+ [Run](#run)

## Background

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
PROJECT="[[YOUR-PROJECT-ID]]"
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

# File: prometheus.yml
# Update audience and target-url values to reflect the Cloud Run service URL
# References to gcp-oidc-token-proxy as a sidecar should remain as localhost
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

# File: deployment.yml
# Update the reference to the ConfigMap name
# Update the reference to the Secret name
DEPLOYMENT=$(mktemp)

sed \
--expression="s|name: CONFIG|name: ${CONFIG}|g" \
--expression="s|secretName: SECRET|secretName: ${SECRET}|g" \
${PWD}/kubernetes/deployment.yml > ${DEPLOYMENT}

kubectl create --filename=${DEPLOYMENT} \
--namespace=${NAMESPACE}

# Await
kubectl wait \
--for=condition=Available \
deployment/prometheus \
--timeout=60s \
--namespace=${NAMESPACE}

# Don't do this in production
# Expose Prometheus on localhost:9090 and the proxy on localhost:7777/metrics
kubectl port-forward deployment/prometheus \
--namespace=${NAMESPACE} \
9090:9090 \
7777:7777
```

Once the deployment completes, you should be able to browse Prometheus' UI on `localhost:9090`

### Docker Compose

Ensure `prometheus.yml` reflects the correct Cloud Run service URL.

> **NOTE** `localhost` values will work when using Docker Compose but it is better to use the services' internal DNS name. In this case, `localhost:9090` becomes `prometheus:9090` and the two occurrences of `localhost:7777` should be replaced by `gcp-oidc-token-proxy:7777`:

```bash
# File: prometheus.yml
# Use Docker Compose internal DNS name to the reference to the Prometheus service
# Use Docker Compose internal DNS name to reference the GCP OIDC Token Proxy service
sed \
--in-place \
--expression="s|localhost:9090|prometheus:9090|g" \
--expression="s|localhost:7777|gcp-oidc-token-proxy:7777|g" \
${PWD}/prometheus.yml
```

And:

```YAML
gcp-oidc-token-proxy:
  restart: always
  depends_on:
  - prometheus
  image: ghcr.io/dazwilkin/gcp-oidc-token-proxy:3994a6a9a8567116a59c30e9a5e9c85418825207
  container_name: gcp-oidc-token-proxy
  command:
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
ghcr.io/dazwilkin/gcp-oidc-token-proxy:3994a6a9a8567116a59c30e9a5e9c85418825207 \
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
    token_url: "http://gcp-oidc-token-proxy:7777/"
    endpoint_params:
      audience: "https://some-service-xxxxxxxxxx-yy.a.run.app"
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

<hr/>
<br/>
<a href="https://www.buymeacoffee.com/dazwilkin" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/default-orange.png" alt="Buy Me A Coffee" height="41" width="174"></a>
