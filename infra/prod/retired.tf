# Import these resources once with retire_legacy_resources=false. The reviewed
# cutover plan uses the default true value, making their deletion explicit.
resource "google_cloud_run_v2_service" "retired_torrent" {
  count = var.retire_legacy_resources ? 0 : 1

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

resource "google_secret_manager_secret" "retired" {
  for_each = var.retire_legacy_resources ? toset([]) : var.legacy_secret_ids

  project   = var.project_id
  secret_id = each.value
  replication {
    auto {}
  }

  lifecycle {
    ignore_changes = all
  }
}
