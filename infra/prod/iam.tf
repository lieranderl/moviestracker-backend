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

resource "google_service_account" "app" {
  project      = var.project_id
  account_id   = "movies-app"
  display_name = "MoviesTracker Qwik application"
}

resource "google_service_account" "gateway" {
  project      = var.project_id
  account_id   = "movies-api-gateway"
  display_name = "MoviesTracker API Gateway"
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

resource "google_project_iam_member" "app_firestore_reader" {
  project = var.project_id
  role    = "roles/datastore.viewer"
  member  = "serviceAccount:${google_service_account.app.email}"
}

resource "google_secret_manager_secret_iam_member" "catalog_tmdb" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.tmdb.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.catalog_jobs.email}"
}

resource "google_service_account_iam_member" "github_act_as" {
  for_each = {
    app             = google_service_account.app.name
    catalog_jobs    = google_service_account.catalog_jobs.name
    gateway         = google_service_account.gateway.name
    imdb_downloader = google_service_account.imdb_downloader.name
    imdb_service    = google_service_account.imdb_service.name
    scheduler       = google_service_account.scheduler.name
  }
  service_account_id = each.value
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:movies-github-deployer@${var.project_id}.iam.gserviceaccount.com"
}
