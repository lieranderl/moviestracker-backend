resource "google_storage_bucket" "tofu_state" {
  name                        = var.state_bucket_name
  location                    = var.region
  project                     = var.project_id
  force_destroy               = false
  public_access_prevention    = "enforced"
  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      age        = 30
      with_state = "ARCHIVED"
    }
    action {
      type = "Delete"
    }
  }
}

locals {
  deployers = {
    backend = {
      account_id    = "movies-backend-deployer"
      display_name  = "MoviesTracker backend release deployer"
      repository_id = var.backend_repository_id
    }
    frontend = {
      account_id    = "movies-frontend-deployer"
      display_name  = "MoviesTracker frontend release deployer"
      repository_id = var.frontend_repository_id
    }
  }
}

resource "google_service_account" "github_deployer" {
  for_each     = local.deployers
  project      = var.project_id
  account_id   = each.value.account_id
  display_name = each.value.display_name
}

resource "google_artifact_registry_repository_iam_member" "github_writer" {
  for_each   = local.deployers
  project    = var.project_id
  location   = var.region
  repository = var.artifact_repository
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.github_deployer[each.key].email}"
}

resource "google_iam_workload_identity_pool" "github" {
  project                   = var.project_id
  workload_identity_pool_id = "moviestracker-github"
  display_name              = "MoviesTracker GitHub"
}

resource "google_iam_workload_identity_pool_provider" "github" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-oidc"
  display_name                       = "GitHub Actions OIDC"
  attribute_mapping = {
    "google.subject"          = "assertion.sub"
    "attribute.repository_id" = "assertion.repository_id"
  }
  attribute_condition = "assertion.repository_id in ['${var.backend_repository_id}', '${var.frontend_repository_id}'] && assertion.ref.matches('^refs/tags/v[0-9]+[.][0-9]+[.][0-9]+$')"
  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account_iam_member" "github_wif" {
  for_each           = local.deployers
  service_account_id = google_service_account.github_deployer[each.key].name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository_id/${each.value.repository_id}"
}

# Read-only drift detection. A separate pool keeps master-branch tokens away
# from the release deployers above, whose bindings match on repository only.
resource "google_service_account" "planner" {
  project      = var.project_id
  account_id   = "movies-infra-planner"
  display_name = "MoviesTracker read-only OpenTofu planner"
}

resource "google_project_iam_member" "planner" {
  for_each = toset([
    "roles/viewer",
    # Reads IAM policies on the project, services, secrets, and buckets.
    "roles/iam.securityReviewer",
  ])
  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.planner.email}"
}

resource "google_storage_bucket_iam_member" "planner_state_reader" {
  bucket = google_storage_bucket.tofu_state.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.planner.email}"
}

resource "google_iam_workload_identity_pool" "github_plan" {
  project                   = var.project_id
  workload_identity_pool_id = "moviestracker-github-plan"
  display_name              = "MoviesTracker GitHub drift"
}

resource "google_iam_workload_identity_pool_provider" "github_plan" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github_plan.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-oidc"
  display_name                       = "GitHub Actions drift OIDC"
  attribute_mapping = {
    "google.subject"          = "assertion.sub"
    "attribute.repository_id" = "assertion.repository_id"
  }
  # Only the drift workflow as merged to master: pull requests cannot run it.
  attribute_condition = "assertion.repository_id == '${var.backend_repository_id}' && assertion.ref == 'refs/heads/master' && assertion.workflow_ref == 'lieranderl/moviestracker-backend/.github/workflows/drift.yml@refs/heads/master'"
  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account_iam_member" "planner_wif" {
  service_account_id = google_service_account.planner.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github_plan.name}/attribute.repository_id/${var.backend_repository_id}"
}

output "state_bucket" {
  value = google_storage_bucket.tofu_state.name
}

output "github_workload_identity_provider" {
  value = google_iam_workload_identity_pool_provider.github.name
}

output "github_backend_deployer_service_account" {
  value = google_service_account.github_deployer["backend"].email
}

output "github_frontend_deployer_service_account" {
  value = google_service_account.github_deployer["frontend"].email
}

output "github_plan_workload_identity_provider" {
  value = google_iam_workload_identity_pool_provider.github_plan.name
}

output "github_planner_service_account" {
  value = google_service_account.planner.email
}
