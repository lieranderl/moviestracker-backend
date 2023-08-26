KZ_LOGIN=jfedotov
KZ_PASSWORD_VERSION=1

gcloud functions deploy get-torrents-for-movie-service \
    --gen2 \
    --region=europe-west1 \
    --runtime=go121 \
    --source=. \
    --entry-point=TorrentsForMovieHandler \
    --set-env-vars RUTOR_SEARCH_URL='http://rutor.is/search/0/0/300/0/%s %s (2160p|1080p)' \
    --set-env-vars KZ_SEARCH_URL='http://kinozal.tv/browse.php?s=%s (1080p|2160p)&g=3&c=0&v=0&d=%s&w=0&t=0&f=0' \
    --set-env-vars KZ_LOGIN=$KZ_LOGIN \
    --set-secrets KZ_PASSWORD=KZ_PASSWORD:$KZ_PASSWORD_VERSION \
    --memory=128Mi \
    --trigger-http \
