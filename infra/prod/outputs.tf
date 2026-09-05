output "github_backend_deployer_service_account" {
  value = "movies-backend-deployer@${var.project_id}.iam.gserviceaccount.com"
}

output "github_frontend_deployer_service_account" {
  value = "movies-frontend-deployer@${var.project_id}.iam.gserviceaccount.com"
}

output "qwik_service_account" {
  value = google_service_account.app.email
}

output "imdb_service_url" {
  value = google_cloud_run_v2_service.imdb.uri
}
