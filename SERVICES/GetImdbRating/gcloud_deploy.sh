FIREBASE_BUCKET=moviestracker-f07e2.appspot.com
FIREBASE_IMDB_FILE=imdb_data.tsv
FIREBASECONFIG_VERSION=3
MAX_RETRY=1
TASK_TIMEOUT=60s
# zip -r getimdbrating.zip imdbrating function.go go.mod 
gcloud functions deploy get-imdb-rating-service \
    --gen2 \
    --region=europe-west1 \
    --runtime=go123 \
    --source=. \
    --entry-point=GetImdbRating \
    --set-env-vars FIREBASE_BUCKET=$FIREBASE_BUCKET \
    --set-env-vars FIREBASE_IMDB_FILE=$FIREBASE_IMDB_FILE \
    --set-secrets FIREBASECONFIG=FIREBASEADMIN_CONFIG:$FIREBASECONFIG_VERSION \
    --memory=128Mi \
    --trigger-http \
    --allow-unauthenticated
