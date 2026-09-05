resource "google_api_gateway_api" "movies" {
  provider     = google-beta
  project      = var.project_id
  api_id       = "moviestracker"
  display_name = "MoviesTracker API"
  depends_on   = [google_project_service.required]
}

resource "google_api_gateway_api_config" "movies" {
  provider      = google-beta
  project       = var.project_id
  api           = google_api_gateway_api.movies.api_id
  api_config_id = var.api_gateway_config_id
  display_name  = "Firestore-only MoviesTracker API"

  gateway_config {
    backend_config {
      google_service_account = google_service_account.gateway.email
    }
  }

  openapi_documents {
    document {
      path = "openapi.yaml"
      contents = base64encode(templatefile("${path.module}/openapi.yaml.tftpl", {
        backend_url = google_cloud_run_v2_service.imdb.uri
      }))
    }
  }

  lifecycle {
    create_before_destroy = true
  }

}

resource "google_api_gateway_gateway" "movies" {
  provider     = google-beta
  project      = var.project_id
  region       = var.region
  gateway_id   = "moviestracker-gw-eu-w1"
  display_name = "MoviesTracker Europe West"
  api_config   = google_api_gateway_api_config.movies.id
}
