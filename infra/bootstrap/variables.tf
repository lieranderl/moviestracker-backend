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

variable "artifact_repository" {
  type    = string
  default = "moviestracker"
}

variable "backend_repository_id" {
  description = "Immutable GitHub repository ID trusted for backend release tags."
  type        = string
  default     = "518156345"
}

variable "frontend_repository_id" {
  description = "Immutable GitHub repository ID trusted for frontend release tags."
  type        = string
  default     = "674721161"
}
