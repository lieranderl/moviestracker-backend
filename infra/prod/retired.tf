# Import these resources once with their retirement switch disabled. Backend
# retirement is independent from the later frontend MongoDB/JWT cutover.
resource "google_cloud_run_v2_service" "retired_torrent" {
  count = var.retire_backend_legacy_resources ? 0 : 1

  project  = var.project_id
  name     = "get-torrents-for-movie-service"
  location = var.region
  template {
    containers {
      image = var.images.getimdbrating
    }
  }

  lifecycle {
    ignore_changes = all
  }
}

resource "google_secret_manager_secret" "retired_kinozal" {
  count = var.retire_backend_legacy_resources ? 0 : 1

  project   = var.project_id
  secret_id = "KZ_PASSWORD"
  replication {
    auto {}
  }

  lifecycle {
    ignore_changes = all
  }
}

resource "google_secret_manager_secret" "retired_frontend" {
  for_each = var.retire_frontend_legacy_resources ? toset([]) : var.frontend_legacy_secret_ids

  project   = var.project_id
  secret_id = each.value
  replication {
    auto {}
  }

  lifecycle {
    ignore_changes = all
  }
}
