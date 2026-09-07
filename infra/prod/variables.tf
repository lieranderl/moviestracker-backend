variable "project_id" {
  type    = string
  default = "moviestracker-f07e2"
}

variable "region" {
  type    = string
  default = "europe-west1"
}

variable "firestore_database" {
  type    = string
  default = "moviestracker"
}

variable "imdb_bucket" {
  type    = string
  default = "moviestracker-imdb-store"
}

variable "artifact_repository" {
  type    = string
  default = "moviestracker"
}

variable "images" {
  description = "Immutable Artifact Registry image URIs keyed by workload."
  type = object({
    frontend         = string
    getmovies        = string
    getimdbrating    = string
    downloadimdbdata = string
  })
  validation {
    condition     = alltrue([for image in values(var.images) : can(regex("@sha256:[0-9a-f]{64}$", image))])
    error_message = "Every image must be pinned by sha256 digest."
  }
}

variable "frontend_url" {
  type    = string
  default = "https://moviestracker.net"
}

variable "google_oauth_client_id" {
  type      = string
  sensitive = true
}

variable "rutor_urls" {
  description = "Comma-separated Rutor source URLs per Firestore collection."
  type = object({
    latesttorrentsmovies = string
    hdr10movies          = string
    dvmovies             = string
  })
  sensitive = true
}

variable "alert_email" {
  type    = string
  default = ""
}

variable "retire_backend_legacy_resources" {
  description = "Destroy the imported torrent service and Kinozal secret during the backend cutover."
  type        = bool
  default     = true
}
