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
    getmovies        = string
    getimdbrating    = string
    downloadimdbdata = string
  })
  validation {
    condition     = alltrue([for image in values(var.images) : can(regex("@sha256:[0-9a-f]{64}$", image))])
    error_message = "Every image must be pinned by sha256 digest."
  }
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

variable "api_gateway_config_id" {
  type    = string
  default = "firestore-v1"
}

variable "retire_backend_legacy_resources" {
  description = "Destroy the imported torrent service and Kinozal secret during the backend cutover."
  type        = bool
  default     = true
}

variable "retire_frontend_legacy_resources" {
  description = "Destroy imported MongoDB and Firebase secrets after the Firestore/JWT frontend is live."
  type        = bool
  default     = false
}

variable "frontend_legacy_secret_ids" {
  type = set(string)
  default = [
    "FIREBASEADMIN_CONFIG",
    "MONGO_INITDB_ROOT_PASSWORD",
    "MONGO_INITDB_ROOT_USERNAME",
    "MONGO_URI",
    "VITE_FIREBASE_CONFIG",
  ]
}
