# EXPORT the following variables:
# export TMDBAPIKEY=''
# export FIREBASE_PROJECT=''
# export FIREBASECONFIG=''
# export MONGO_URI=''
IMAGE_URL=europe-west1-docker.pkg.dev/moviestracker-f07e2/gcf-artifacts/getmovies-image
FIREBASE_PROJECT='moviestracker-f07e2'
RUTOR_URLS="http://rutor.is/search/0/1/300/0/.*(2021|2022|2023|2024|2025).*.(2160p%7C1080p).*"
RUTOR_HDR10_URLS="http://rutor.is/search/0/1/300/0/HDR10,http://rutor.is/search/1/1/300/0/HDR10,http://rutor.is/search/0/5/300/0/HDR10,http://rutor.is/search/0/7/300/0/HDR10"
RUTOR_DV_URLS="http://rutor.is/search/0/1/300/0/dolby%20vision,http://rutor.is/search/1/1/300/0/dolby%20vision,http://rutor.is/search/2/1/300/0/dolby%20vision,http://rutor.is/search/0/5/300/0/dolby%20vision,http://rutor.is/search/0/7/300/0/dolby%20vision"
FIREBASECONFIG_VERSION=3
MAX_RETRY=1
TASK_TIMEOUT=400s


if [ -z "$1" ]; then
    echo "No argument supplied; please provide a database name: latesttorrentsmovies, hdr10movies, dvmovies"
    exit 1
fi

DBNAME=$1

if [ -z "$2" ]; then
    echo "No argument supplied; please provide a database type: firebase, mongo"
    exit 1
fi

DBTYPE=$2

if [ -z "$3" ]; then
    echo "No argument supplied; please provide true or flase for image build"
    exit 1
fi

if [ "$3" = true ]; then
    echo "Building image..."
    gcloud builds submit --tag $IMAGE_URL
fi

if ! gcloud run jobs create getmovies-$DBNAME-job \
    --image $IMAGE_URL \
    --args $DBNAME \
    --task-timeout $TASK_TIMEOUT \
    --max-retries $MAX_RETRY \
    --set-secrets FIREBASECONFIG=FIREBASEADMIN_CONFIG:$FIREBASECONFIG_VERSION \
    --set-secrets TMDBAPIKEY=TMDBAPIKEY:latest \
    --set-secrets MONGO_URI=MONGO_URI:latest \
    --set-env-vars DBNAME=$DBNAME \
    --set-env-vars DBTYPE=$DBTYPE \
    --set-env-vars RUTOR_URLS=$RUTOR_URLS \
    --set-env-vars ^##^RUTOR_HDR10_URLS=$RUTOR_HDR10_URLS \
    --set-env-vars ^##^RUTOR_DV_URLS=$RUTOR_DV_URLS \
    --set-env-vars FIREBASE_PROJECT=$FIREBASE_PROJECT; then
    echo "job already exists, updating..."
    gcloud run jobs update getmovies-$DBNAME-job --clear-env-vars
    gcloud run jobs update getmovies-$DBNAME-job \
        --image $IMAGE_URL \
        --args $DBNAME \
        --args $DBTYPE \
        --task-timeout $TASK_TIMEOUT \
        --max-retries $MAX_RETRY \
        --update-secrets FIREBASECONFIG=FIREBASEADMIN_CONFIG:$FIREBASECONFIG_VERSION \
        --update-secrets TMDBAPIKEY=TMDBAPIKEY:latest \
        --update-secrets MONGO_URI=MONGO_URI:latest \
        --update-env-vars DBNAME=$DBNAME \
        --update-env-vars DBTYPE=$DBTYPE \
        --update-env-vars RUTOR_URLS=$RUTOR_URLS \
        --update-env-vars ^##^RUTOR_HDR10_URLS=$RUTOR_HDR10_URLS \
        --update-env-vars ^##^RUTOR_DV_URLS=$RUTOR_DV_URLS \
        --update-env-vars FIREBASE_PROJECT=$FIREBASE_PROJECT

fi
