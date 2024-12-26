IMAGE_URL=europe-west1-docker.pkg.dev/moviestracker-f07e2/gcf-artifacts/get-imdb-rating-service
FIREBASE_BUCKET=moviestracker-f07e2.appspot.com
MAX_RETRY=1
TASK_TIMEOUT=20s

gcloud builds submit --tag $IMAGE_URL --region=europe-west1
gcloud run services update get-imdb-rating-service \
    --image $IMAGE_URL:latest \
    --timeout $TASK_TIMEOUT \
    --concurrency 20 \
    --max-instances 2 \
    --service-min-instances 0 \
    --add-volume=name=imdb-mount,type=cloud-storage,bucket=$FIREBASE_BUCKET \
    --add-volume-mount=volume=imdb-mount,mount-path=/app/store \
    --port 8080 \
    --region=europe-west1