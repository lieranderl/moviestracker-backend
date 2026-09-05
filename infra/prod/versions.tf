terraform {
  required_version = "= 1.12.6"

  backend "gcs" {
    bucket = "moviestracker-f07e2-tofu-state"
    prefix = "backend/prod"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "= 7.42.0"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "= 7.42.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

provider "google-beta" {
  project = var.project_id
  region  = var.region
}
