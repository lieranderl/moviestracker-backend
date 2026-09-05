output "github_deployer_service_account" {
  value = "movies-github-deployer@${var.project_id}.iam.gserviceaccount.com"
}

output "qwik_service_account" {
  value = google_service_account.app.email
}

output "imdb_service_url" {
  value = google_cloud_run_v2_service.imdb.uri
}
