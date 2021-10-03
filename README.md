
```bash
BILLING=$(gcloud alpha billing accounts list --format="value(name)")
PROJECT="dazwilkin-$(date +%y%m%d)-oauthproxy"
REGION="us-west2"
REPOSITORY="repo"

GHCR_TOKEN="..."

gcloud projects create ${PROJECT}
gcloud beta billing projects link ${PROJECT} --billing-account=${BILLING}

for SERVICE in "run" "artifactregistry"
do
  gcloud services enable ${SERVICE}.googleapis.com \
  --project=${PROJECT}
done

gcloud artifacts repositories create ${REPOSITORY} \
--repository-format=docker \
--location=${REGION} \
--project=${PROJECT}

GHCR="ghcr.io/brabantcourt"
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
