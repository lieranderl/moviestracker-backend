IMAGE_URL=europe-west1-docker.pkg.dev/moviestracker-f07e2/gcf-artifacts/downloadimdbdata-image
FIREBASE_BUCKET=moviestracker-f07e2.appspot.com
FIREBASE_IMDB_FILE=imdb_data.tsv
FIREBASECONFIG_VERSION=3
MAX_RETRY=1
TASK_TIMEOUT=60s

gcloud builds submit --tag $IMAGE_URL
if ! gcloud run jobs create downloadimdbdata-job \
    --image $IMAGE_URL \
    --task-timeout $TASK_TIMEOUT \
    --max-retries $MAX_RETRY \
    --set-secrets FIREBASECONFIG=FIREBASEADMIN_CONFIG:$FIREBASECONFIG_VERSION \
    --set-env-vars FIREBASE_BUCKET=$FIREBASE_BUCKET \
    --set-env-vars FIREBASE_IMDB_FILE=$FIREBASE_IMDB_FILE; then
    echo "job already exists, updating..."
    gcloud run jobs update downloadimdbdata-job \
        --image $IMAGE_URL \
        --task-timeout $TASK_TIMEOUT \
        --max-retries $MAX_RETRY \
        --update-secrets FIREBASECONFIG=FIREBASEADMIN_CONFIG:$FIREBASECONFIG_VERSION \
        --update-env-vars FIREBASE_BUCKET=$FIREBASE_BUCKET \
        --update-env-vars FIREBASE_IMDB_FILE=$FIREBASE_IMDB_FILE
fi
