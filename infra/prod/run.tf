resource "google_cloud_run_v2_service" "imdb" {
  project             = var.project_id
  name                = "get-imdb-rating-service"
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = true
  # Public by decision: installed moviestracker-app clients (desktop, NAS,
  # web) call /getimdb without credentials. max_instance_count bounds the cost
  # of abuse. The invoker bindings below stay for authenticated callers.
  invoker_iam_disabled = true

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

  lifecycle {
    ignore_changes = [
      client,
      client_version,
      template[0].containers[0].image,
    ]
  }

  depends_on = [
    google_project_service.required,
    google_service_account_iam_member.github_backend_act_as,
  ]
}

# Deployed by the moviestracker-app repository (web.yml): it moves the image,
# revision, and traffic; OpenTofu owns identity, scaling, and configuration.
resource "google_cloud_run_v2_service" "web" {
  project             = var.project_id
  name                = "moviestracker-web"
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = true

  template {
    labels = {
      app = "moviestracker-web"
    }
    service_account                  = google_service_account.web.email
    timeout                          = "3600s"
    max_instance_request_concurrency = 250
    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }
    containers {
      image = var.images.frontend
      ports {
        name           = "http1"
        container_port = 8080
      }
      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
        cpu_idle          = true
        startup_cpu_boost = true
      }
      env {
        name  = "MT_WEB_BASE_URL"
        value = var.frontend_url
      }
      env {
        name  = "MT_WEB_GOOGLE_CLIENT_ID"
        value = var.google_oauth_client_id
      }
      env {
        name = "MT_WEB_GOOGLE_CLIENT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.runtime["GOOGLE_SECRET"].secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "MT_WEB_SESSION_KEY"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.runtime["WEB_SESSION_KEY"].secret_id
            version = "latest"
          }
        }
      }
      env {
        name  = "MT_WEB_FIRESTORE_PROJECT"
        value = var.project_id
      }
      env {
        name  = "MT_WEB_FIRESTORE_DATABASE"
        value = google_firestore_database.movies.name
      }
      env {
        name = "MT_WEB_TMDB_KEY"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.runtime["TMDB_API_KEY"].secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "MT_WEB_JACRED_KEY"
        value_source {
          secret_key_ref {
            secret = google_secret_manager_secret.runtime["JACRED_APIKEY"].secret_id
            # Pinned as deployed; move to "latest" together with the web repo.
            version = "1"
          }
        }
      }
      # Cloud Run's default TCP probe, as deployed. The web workflow already
      # gates traffic on /readyz.
      startup_probe {
        timeout_seconds   = 240
        period_seconds    = 240
        failure_threshold = 1
        tcp_socket {
          port = 8080
        }
      }
    }
  }

  lifecycle {
    ignore_changes = [
      client,
      client_version,
      template[0].containers[0].image,
      template[0].revision,
      traffic,
    ]
  }

  depends_on = [
    google_project_service.required,
    google_secret_manager_secret_iam_member.web_secrets,
    google_service_account_iam_member.web_deployer_act_as,
  ]
}

resource "google_cloud_run_v2_service_iam_member" "web_public_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.web.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_cloud_run_v2_service_iam_member" "downloader_imdb_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.imdb.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.imdb_downloader.email}"
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
          name  = "ENRICHER_CONCURRENCY"
          value = "10"
        }
        env {
          name  = each.key == "latesttorrentsmovies" ? "RUTOR_URLS" : each.key == "hdr10movies" ? "RUTOR_HDR10_URLS" : "RUTOR_DV_URLS"
          value = lookup(var.rutor_urls, each.key)
        }
        env {
          name = "TMDBAPIKEY"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.runtime["TMDB_API_KEY"].secret_id
              version = "latest"
            }
          }
        }
      }
    }
  }


  lifecycle {
    ignore_changes = [
      client,
      client_version,
      template[0].template[0].containers[0].image,
    ]
  }

  depends_on = [
    google_project_service.required,
    google_service_account_iam_member.github_backend_act_as,
  ]
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
        env {
          name  = "IMDB_SERVICE_URL"
          value = google_cloud_run_v2_service.imdb.uri
        }
      }
    }
  }

  lifecycle {
    ignore_changes = [
      client,
      client_version,
      template[0].template[0].containers[0].image,
    ]
  }

  depends_on = [
    google_project_service.required,
    google_service_account_iam_member.github_backend_act_as,
  ]
}

resource "google_cloud_run_v2_job_iam_member" "scheduler_catalog_invoker" {
  for_each = local.catalog_jobs
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.catalog[each.key].name
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

resource "google_cloud_run_v2_job_iam_member" "github_catalog_invoker" {
  for_each = local.catalog_jobs
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.catalog[each.key].name
  role     = "roles/run.invoker"
  member   = "serviceAccount:movies-backend-deployer@${var.project_id}.iam.gserviceaccount.com"
}

resource "google_cloud_run_v2_job_iam_member" "github_imdb_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.imdb_downloader.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:movies-backend-deployer@${var.project_id}.iam.gserviceaccount.com"
}

resource "google_cloud_run_v2_service_iam_member" "github_backend_developer" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.imdb.name
  role     = "roles/run.developer"
  member   = "serviceAccount:movies-backend-deployer@${var.project_id}.iam.gserviceaccount.com"
}

resource "google_cloud_run_v2_service_iam_member" "github_backend_imdb_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.imdb.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:movies-backend-deployer@${var.project_id}.iam.gserviceaccount.com"
}

resource "google_cloud_run_v2_job_iam_member" "github_catalog_developer" {
  for_each = local.catalog_jobs
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.catalog[each.key].name
  role     = "roles/run.developer"
  member   = "serviceAccount:movies-backend-deployer@${var.project_id}.iam.gserviceaccount.com"
}

resource "google_cloud_run_v2_job_iam_member" "github_imdb_developer" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.imdb_downloader.name
  role     = "roles/run.developer"
  member   = "serviceAccount:movies-backend-deployer@${var.project_id}.iam.gserviceaccount.com"
}
