IMAGE_URL=europe-west1-docker.pkg.dev/moviestracker-f07e2/gcf-artifacts/downloadimdbdata-image
FIREBASE_BUCKET=moviestracker-f07e2.appspot.com
MAX_RETRY=1
TASK_TIMEOUT=60s

gcloud builds submit --tag $IMAGE_URL --region=europe-west1
if ! gcloud run jobs create downloadimdbdata-job \
    --image $IMAGE_URL:latest \
    --task-timeout $TASK_TIMEOUT \
    --max-retries $MAX_RETRY \
    --add-volume=name=imdb-mount,type=cloud-storage,bucket=$FIREBASE_BUCKET \
    --add-volume-mount=volume=imdb-mount,mount-path=/app/store \
    --region=europe-west1; then
    echo "job already exists, updating..."
    gcloud run jobs update downloadimdbdata-job \
        --image $IMAGE_URL\:latest \
        --task-timeout $TASK_TIMEOUT \
        --max-retries $MAX_RETRY \
        --add-volume=name=imdb-mount,type=cloud-storage,bucket=$FIREBASE_BUCKET \
        --add-volume-mount=volume=imdb-mount,mount-path=/app/store \
        --region=europe-west1
fi
