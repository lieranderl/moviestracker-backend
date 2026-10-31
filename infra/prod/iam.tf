resource "google_service_account" "imdb_service" {
  project      = var.project_id
  account_id   = "movies-imdb-service"
  display_name = "MoviesTracker IMDb service"
}

resource "google_service_account" "imdb_downloader" {
  project      = var.project_id
  account_id   = "movies-imdb-downloader"
  display_name = "MoviesTracker IMDb downloader"
}

resource "google_service_account" "catalog_jobs" {
  project      = var.project_id
  account_id   = "movies-catalog-jobs"
  display_name = "MoviesTracker catalog jobs"
}

resource "google_service_account" "scheduler" {
  project      = var.project_id
  account_id   = "movies-scheduler"
  display_name = "MoviesTracker scheduler"
}

resource "google_service_account" "web" {
  project      = var.project_id
  account_id   = "movies-web"
  display_name = "MoviesTracker web application (Go)"
}

resource "google_storage_bucket_iam_member" "imdb_reader" {
  bucket = google_storage_bucket.imdb.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.imdb_service.email}"
}

resource "google_storage_bucket_iam_member" "imdb_writer" {
  bucket = google_storage_bucket.imdb.name
  role   = "roles/storage.objectUser"
  member = "serviceAccount:${google_service_account.imdb_downloader.email}"
}

resource "google_project_iam_member" "catalog_firestore_writer" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.catalog_jobs.email}"
}

# The web app stores per-user state, so it needs read-write Firestore access.
resource "google_project_iam_member" "web_firestore_user" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.web.email}"
}

resource "google_secret_manager_secret_iam_member" "catalog_tmdb_api_key" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.runtime["TMDB_API_KEY"].secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.catalog_jobs.email}"
}

locals {
  # Every runtime secret in the project. Values are written out of band; only
  # the containers are managed here. AUTH_SECRET, the retired Qwik frontend's
  # session secret, had no consumer and was removed on 2026-10-31; do not
  # recreate it.
  runtime_secret_ids = toset(["GOOGLE_SECRET", "JACRED_APIKEY", "TMDB_API_KEY", "WEB_SESSION_KEY"])
  web_secret_ids     = toset(["GOOGLE_SECRET", "JACRED_APIKEY", "TMDB_API_KEY", "WEB_SESSION_KEY"])
}

resource "google_secret_manager_secret" "runtime" {
  for_each  = local.runtime_secret_ids
  project   = var.project_id
  secret_id = each.value
  replication {
    auto {}
  }
  depends_on = [google_project_service.required]
}

resource "google_secret_manager_secret_iam_member" "web_secrets" {
  for_each  = local.web_secret_ids
  project   = var.project_id
  secret_id = google_secret_manager_secret.runtime[each.value].secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.web.email}"
}

resource "google_service_account_iam_member" "github_backend_act_as" {
  for_each = {
    catalog_jobs    = google_service_account.catalog_jobs.name
    imdb_downloader = google_service_account.imdb_downloader.name
    imdb_service    = google_service_account.imdb_service.name
  }
  service_account_id = each.value
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:movies-backend-deployer@${var.project_id}.iam.gserviceaccount.com"
}

# The web repository's release identity deploys moviestracker-web as movies-web.
resource "google_service_account_iam_member" "web_deployer_act_as" {
  service_account_id = google_service_account.web.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:movies-web-deployer@${var.project_id}.iam.gserviceaccount.com"
}
