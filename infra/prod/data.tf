data "google_project" "current" {
  project_id = var.project_id
}

resource "google_firestore_database" "movies" {
  project                           = var.project_id
  name                              = var.firestore_database
  location_id                       = var.region
  type                              = "FIRESTORE_NATIVE"
  concurrency_mode                  = "PESSIMISTIC"
  delete_protection_state           = "DELETE_PROTECTION_ENABLED"
  point_in_time_recovery_enablement = "POINT_IN_TIME_RECOVERY_DISABLED"
  deletion_policy                   = "ABANDON"

  depends_on = [google_project_service.required]
}

resource "google_storage_bucket" "imdb" {
  name                        = var.imdb_bucket
  project                     = var.project_id
  location                    = var.region
  force_destroy               = false
  public_access_prevention    = "enforced"
  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      age        = 14
      with_state = "ARCHIVED"
    }
    action {
      type = "Delete"
    }
  }
}

resource "google_artifact_registry_repository" "movies" {
  project       = var.project_id
  location      = var.region
  repository_id = var.artifact_repository
  format        = "DOCKER"

  cleanup_policy_dry_run = false
  cleanup_policies {
    id     = "delete-old-images"
    action = "DELETE"
    condition {
      older_than = "2592000s"
    }
  }
  cleanup_policies {
    id     = "keep-recent-images"
    action = "KEEP"
    most_recent_versions {
      keep_count = 10
    }
  }

  depends_on = [google_project_service.required]
}
