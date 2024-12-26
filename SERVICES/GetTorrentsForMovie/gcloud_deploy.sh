IMAGE_URL=europe-west1-docker.pkg.dev/moviestracker-f07e2/gcf-artifacts/get-torrents-for-movie-service
MAX_RETRY=1
TASK_TIMEOUT=60s
KZ_LOGIN=jfedotov
KZ_PASSWORD_VERSION=1

gcloud builds submit --tag $IMAGE_URL --region=europe-west1
gcloud run services update get-torrents-for-movie-service \
    --image $IMAGE_URL:latest \
    --timeout $TASK_TIMEOUT \
    --set-env-vars RUTOR_SEARCH_URL='http://rutor.is/search/0/0/300/0/%s %s (2160p|1080p)' \
    --set-env-vars KZ_SEARCH_URL='http://kinozal.tv/browse.php?s=%s (1080p|2160p)&g=3&c=0&v=0&d=%s&w=0&t=0&f=0' \
    --set-env-vars KZ_LOGIN=$KZ_LOGIN \
    --set-secrets KZ_PASSWORD=KZ_PASSWORD:$KZ_PASSWORD_VERSION \
    --port 8080 \
    --region=europe-west1
