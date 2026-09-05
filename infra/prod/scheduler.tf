resource "google_cloud_scheduler_job" "jobs" {
  for_each = local.scheduler_jobs

  project          = var.project_id
  region           = var.region
  name             = each.key
  schedule         = each.value.schedule
  time_zone        = "Europe/Brussels"
  attempt_deadline = "320s"

  retry_config {
    retry_count          = 1
    max_retry_duration   = "300s"
    min_backoff_duration = "30s"
    max_backoff_duration = "60s"
    max_doublings        = 1
  }

  http_target {
    http_method = "POST"
    uri         = "https://${var.region}-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/${var.project_id}/jobs/${each.value.job_name}:run"
    oauth_token {
      service_account_email = google_service_account.scheduler.email
      scope                 = "https://www.googleapis.com/auth/cloud-platform"
    }
  }

  depends_on = [
    google_cloud_run_v2_job_iam_member.scheduler_catalog_invoker,
    google_cloud_run_v2_job_iam_member.scheduler_imdb_invoker,
  ]
}
