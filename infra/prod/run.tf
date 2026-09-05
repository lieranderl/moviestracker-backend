resource "google_cloud_run_v2_service" "imdb" {
  project             = var.project_id
  name                = "get-imdb-rating-service"
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = true

  template {
    service_account                  = google_service_account.imdb_service.email
    timeout                          = "30s"
    max_instance_request_concurrency = 40

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = var.images.getimdbrating
      ports {
        container_port = 8080
      }
      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
        cpu_idle = true
      }
      env {
        name  = "IMDB_BUCKET"
        value = google_storage_bucket.imdb.name
      }
      env {
        name  = "IMDB_OBJECT"
        value = "title.ratings.tsv.gz"
      }
      env {
        name  = "IMDB_REFRESH_INTERVAL"
        value = "15m"
      }
      startup_probe {
        initial_delay_seconds = 1
        timeout_seconds       = 2
        period_seconds        = 3
        failure_threshold     = 20
        http_get {
          path = "/readyz"
          port = 8080
        }
      }
      liveness_probe {
        timeout_seconds   = 2
        period_seconds    = 30
        failure_threshold = 3
        http_get {
          path = "/healthz"
          port = 8080
        }
      }
    }
  }

  depends_on = [google_project_service.required]
}

resource "google_cloud_run_v2_service_iam_member" "gateway_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.imdb.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.gateway.email}"
}

resource "google_cloud_run_v2_job" "catalog" {
  for_each = local.catalog_jobs

  project             = var.project_id
  name                = "getmovies-${each.key}-job"
  location            = var.region
  deletion_protection = true

  template {
    parallelism = 1
    task_count  = 1
    template {
      service_account = google_service_account.catalog_jobs.email
      timeout         = "600s"
      max_retries     = 1
      containers {
        image = var.images.getmovies
        args  = [each.key]
        resources {
          limits = {
            cpu    = "1"
            memory = "512Mi"
          }
        }
        env {
          name  = "GCP_PROJECT"
          value = var.project_id
        }
        env {
          name  = "FIRESTORE_DATABASE"
          value = google_firestore_database.movies.name
        }
        env {
          name  = "JOB_TIMEOUT"
          value = "10m"
        }
        env {
          name  = each.key == "latesttorrentsmovies" ? "RUTOR_URLS" : each.key == "hdr10movies" ? "RUTOR_HDR10_URLS" : "RUTOR_DV_URLS"
          value = lookup(var.rutor_urls, each.key)
        }
        env {
          name = "TMDBAPIKEY"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.tmdb.secret_id
              version = "latest"
            }
          }
        }
      }
    }
  }

  depends_on = [google_project_service.required]
}

resource "google_cloud_run_v2_job" "imdb_downloader" {
  project             = var.project_id
  name                = "downloadimdbdata-job"
  location            = var.region
  deletion_protection = true

  template {
    parallelism = 1
    task_count  = 1
    template {
      service_account = google_service_account.imdb_downloader.email
      timeout         = "300s"
      max_retries     = 1
      containers {
        image = var.images.downloadimdbdata
        resources {
          limits = {
            cpu    = "1"
            memory = "512Mi"
          }
        }
        env {
          name  = "IMDB_BUCKET"
          value = google_storage_bucket.imdb.name
        }
        env {
          name  = "IMDB_OBJECT"
          value = "title.ratings.tsv.gz"
        }
        env {
          name  = "JOB_TIMEOUT"
          value = "5m"
        }
      }
    }
  }

  depends_on = [google_project_service.required]
}

resource "google_cloud_run_v2_job_iam_member" "scheduler_catalog_invoker" {
  for_each = google_cloud_run_v2_job.catalog
  project  = var.project_id
  location = var.region
  name     = each.value.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.scheduler.email}"
}

resource "google_cloud_run_v2_job_iam_member" "scheduler_imdb_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.imdb_downloader.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.scheduler.email}"
}
