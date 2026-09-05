variable "project_id" {
  type    = string
  default = "moviestracker-f07e2"
}

variable "region" {
  type    = string
  default = "europe-west1"
}

variable "state_bucket_name" {
  type    = string
  default = "moviestracker-f07e2-tofu-state"
}

variable "github_repositories" {
  description = "GitHub repositories trusted for release-tag deployments."
  type        = set(string)
  default = [
    "lieranderl/moviestracker-backend",
    "lieranderl/qwik-moviestracker",
  ]
}
