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

# moviestracker-web, its identity, and its secrets were created outside
# OpenTofu by the moviestracker-app repository; adopt them as deployed.
import {
  to = google_cloud_run_v2_service.web
  id = "projects/moviestracker-f07e2/locations/europe-west1/services/moviestracker-web"
}

import {
  to = google_cloud_run_v2_service_iam_member.web_public_invoker
  id = "projects/moviestracker-f07e2/locations/europe-west1/services/moviestracker-web roles/run.invoker allUsers"
}

import {
  to = google_service_account.web
  id = "projects/moviestracker-f07e2/serviceAccounts/movies-web@moviestracker-f07e2.iam.gserviceaccount.com"
}

import {
  to = google_service_account_iam_member.web_deployer_act_as
  id = "projects/moviestracker-f07e2/serviceAccounts/movies-web@moviestracker-f07e2.iam.gserviceaccount.com roles/iam.serviceAccountUser serviceAccount:movies-web-deployer@moviestracker-f07e2.iam.gserviceaccount.com"
}

import {
  to = google_project_iam_member.web_firestore_user
  id = "moviestracker-f07e2 roles/datastore.user serviceAccount:movies-web@moviestracker-f07e2.iam.gserviceaccount.com"
}

import {
  for_each = toset(["JACRED_APIKEY", "WEB_SESSION_KEY"])
  to       = google_secret_manager_secret.runtime[each.key]
  id       = "projects/moviestracker-f07e2/secrets/${each.key}"
}

import {
  for_each = local.web_secret_ids
  to       = google_secret_manager_secret_iam_member.web_secrets[each.key]
  id       = "projects/moviestracker-f07e2/secrets/${each.key} roles/secretmanager.secretAccessor serviceAccount:movies-web@moviestracker-f07e2.iam.gserviceaccount.com"
}

# Catalog jobs read TMDB_API_KEY since TMDBAPIKEY was deleted out of band.
import {
  to = google_secret_manager_secret_iam_member.catalog_tmdb_api_key
  id = "projects/moviestracker-f07e2/secrets/TMDB_API_KEY roles/secretmanager.secretAccessor serviceAccount:movies-catalog-jobs@moviestracker-f07e2.iam.gserviceaccount.com"
}
