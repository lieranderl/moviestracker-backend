import {
  to = google_firestore_database.movies
  id = "projects/moviestracker-f07e2/databases/moviestracker"
}

import {
  to = google_storage_bucket.imdb
  id = "moviestracker-imdb-store"
}

import {
  to = google_artifact_registry_repository.movies
  id = "projects/moviestracker-f07e2/locations/europe-west1/repositories/moviestracker"
}

import {
  to = google_secret_manager_secret.tmdb
  id = "projects/moviestracker-f07e2/secrets/TMDBAPIKEY"
}

import {
  to = google_cloud_run_v2_service.imdb
  id = "projects/moviestracker-f07e2/locations/europe-west1/services/get-imdb-rating-service"
}

import {
  for_each = local.catalog_jobs
  to       = google_cloud_run_v2_job.catalog[each.key]
  id       = "projects/moviestracker-f07e2/locations/europe-west1/jobs/getmovies-${each.key}-job"
}

import {
  to = google_cloud_run_v2_job.imdb_downloader
  id = "projects/moviestracker-f07e2/locations/europe-west1/jobs/downloadimdbdata-job"
}

import {
  for_each = local.scheduler_jobs
  to       = google_cloud_scheduler_job.jobs[each.key]
  id       = "projects/moviestracker-f07e2/locations/europe-west1/jobs/${each.key}"
}

import {
  to = google_api_gateway_api.movies
  id = "projects/moviestracker-f07e2/locations/global/apis/moviestracker"
}

import {
  to = google_api_gateway_gateway.movies
  id = "projects/moviestracker-f07e2/locations/europe-west1/gateways/moviestracker-gw-eu-w1"
}
