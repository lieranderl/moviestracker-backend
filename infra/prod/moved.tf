# State transitions for the 2026 frontend replacement. moviestracker-app (Qwik)
# was replaced by moviestracker-web and the backend TMDBAPIKEY secret was
# folded into TMDB_API_KEY, both outside OpenTofu.

moved {
  from = google_secret_manager_secret.frontend
  to   = google_secret_manager_secret.runtime
}

# Already deleted in production: forget them instead of recreating them.
removed {
  from = google_cloud_run_v2_service.frontend
  lifecycle {
    destroy = false
  }
}

removed {
  from = google_cloud_run_v2_service_iam_member.frontend_public_invoker
  lifecycle {
    destroy = false
  }
}

removed {
  from = google_cloud_run_v2_service_iam_member.github_frontend_developer
  lifecycle {
    destroy = false
  }
}

removed {
  from = google_secret_manager_secret.tmdb
  lifecycle {
    destroy = false
  }
}

# This binding was on the deleted TMDBAPIKEY secret; catalog_tmdb_api_key
# adopts the live binding on TMDB_API_KEY.
removed {
  from = google_secret_manager_secret_iam_member.catalog_tmdb
  lifecycle {
    destroy = false
  }
}
