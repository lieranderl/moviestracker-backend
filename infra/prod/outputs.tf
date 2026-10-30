output "github_backend_deployer_service_account" {
  value = "movies-backend-deployer@${var.project_id}.iam.gserviceaccount.com"
}

output "web_deployer_service_account" {
  value = "movies-web-deployer@${var.project_id}.iam.gserviceaccount.com"
}

output "web_service_account" {
  value = google_service_account.web.email
}

output "imdb_service_url" {
  value = google_cloud_run_v2_service.imdb.uri
}
