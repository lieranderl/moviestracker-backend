locals {
  notification_channels = var.alert_email == "" ? [] : [google_monitoring_notification_channel.email[0].name]
}

resource "google_monitoring_notification_channel" "email" {
  count        = var.alert_email == "" ? 0 : 1
  project      = var.project_id
  display_name = "MoviesTracker operations"
  type         = "email"
  labels = {
    email_address = var.alert_email
  }
}

resource "google_logging_metric" "catalog_success" {
  project = var.project_id
  name    = "moviestracker/catalog_job_success"
  filter  = <<-EOT
    resource.type="cloud_run_job"
    resource.labels.job_name=~"getmovies-.*-job"
    jsonPayload.event="catalog_pipeline_completed"
  EOT
  label_extractors = {
    job_name = "EXTRACT(resource.labels.job_name)"
  }
  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    labels {
      key        = "job_name"
      value_type = "STRING"
    }
  }
}

resource "google_logging_metric" "catalog_degraded" {
  project = var.project_id
  name    = "moviestracker/catalog_job_degraded"
  filter  = <<-EOT
    resource.type="cloud_run_job"
    resource.labels.job_name=~"getmovies-.*-job"
    jsonPayload.event="catalog_pipeline_completed"
    (jsonPayload.failed>0 OR jsonPayload.rejected_parsing>0)
  EOT
  label_extractors = {
    job_name = "EXTRACT(resource.labels.job_name)"
  }
  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    labels {
      key        = "job_name"
      value_type = "STRING"
    }
  }
}

resource "google_logging_metric" "imdb_download_success" {
  project = var.project_id
  name    = "moviestracker/imdb_download_success"
  filter  = <<-EOT
    resource.type="cloud_run_job"
    resource.labels.job_name="downloadimdbdata-job"
    jsonPayload.event="imdb_ratings_updated"
  EOT
  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
  }
}

resource "google_logging_metric" "imdb_refresh_failure" {
  project = var.project_id
  name    = "moviestracker/imdb_refresh_failure"
  filter  = <<-EOT
    resource.type="cloud_run_revision"
    resource.labels.service_name="get-imdb-rating-service"
    jsonPayload.event="imdb_index_refresh_failed"
  EOT
  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
  }
}

resource "google_monitoring_alert_policy" "scheduled_execution_failure" {
  project               = var.project_id
  display_name          = "MoviesTracker scheduled execution failed"
  combiner              = "OR"
  notification_channels = local.notification_channels
  conditions {
    display_name = "Cloud Run job completed unsuccessfully"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_job\" AND metric.type = \"run.googleapis.com/job/completed_execution_count\" AND metric.label.result = \"failed\""
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "0s"
      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_SUM"
      }
    }
  }
  alert_strategy {
    auto_close = "1800s"
  }
}

resource "google_monitoring_alert_policy" "catalog_stale" {
  project               = var.project_id
  display_name          = "MoviesTracker catalog data is stale"
  combiner              = "OR"
  notification_channels = local.notification_channels
  conditions {
    display_name = "No successful catalog job within 24 hours"
    condition_absent {
      filter   = "resource.type = \"cloud_run_job\" AND metric.type = \"logging.googleapis.com/user/${google_logging_metric.catalog_success.name}\""
      duration = "82800s"
      aggregations {
        alignment_period     = "3600s"
        per_series_aligner   = "ALIGN_SUM"
        cross_series_reducer = "REDUCE_SUM"
        group_by_fields      = ["metric.label.job_name"]
      }
    }
  }
}

resource "google_monitoring_alert_policy" "catalog_degraded" {
  project               = var.project_id
  display_name          = "MoviesTracker catalog run degraded"
  combiner              = "OR"
  notification_channels = local.notification_channels
  conditions {
    display_name = "Catalog run contains parsing or upstream failures"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_job\" AND metric.type = \"logging.googleapis.com/user/${google_logging_metric.catalog_degraded.name}\""
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "0s"
      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_SUM"
        cross_series_reducer = "REDUCE_SUM"
        group_by_fields      = ["metric.label.job_name"]
      }
    }
  }
  alert_strategy {
    auto_close = "1800s"
  }
}

resource "google_monitoring_alert_policy" "imdb_stale" {
  project               = var.project_id
  display_name          = "MoviesTracker IMDb dataset is stale"
  combiner              = "OR"
  notification_channels = local.notification_channels
  conditions {
    display_name = "No successful IMDb download within 25 hours including grace"
    condition_prometheus_query_language {
      # IMDb runs daily in UTC. Cloud Monitoring limits the combined lookback
      # and retest window to 25h: 24h45m + 15m, covering runtime/logging jitter.
      # Sum DELTA counts, including isolated successes; missing series mean zero.
      query               = <<-EOT
        (
          sum(sum_over_time({__name__="logging.googleapis.com/user/${google_logging_metric.imdb_download_success.name}", monitored_resource="cloud_run_job", project_id="${var.project_id}", job_name="downloadimdbdata-job"}[24h45m]))
          or vector(0)
        ) == 0
      EOT
      duration            = "900s"
      evaluation_interval = "60s"
    }
  }
}

resource "google_monitoring_alert_policy" "imdb_refresh_failures" {
  project               = var.project_id
  display_name          = "MoviesTracker repeated IMDb refresh failures"
  combiner              = "OR"
  notification_channels = local.notification_channels
  conditions {
    display_name = "Three refresh failures in 15 minutes"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND metric.type = \"logging.googleapis.com/user/${google_logging_metric.imdb_refresh_failure.name}\""
      comparison      = "COMPARISON_GT"
      threshold_value = 2
      duration        = "0s"
      aggregations {
        alignment_period   = "900s"
        per_series_aligner = "ALIGN_SUM"
      }
    }
  }
}

resource "google_monitoring_alert_policy" "imdb_5xx" {
  project               = var.project_id
  display_name          = "MoviesTracker IMDb service 5xx rate"
  combiner              = "OR"
  notification_channels = local.notification_channels
  conditions {
    display_name = "5xx responses exceed 2 percent"
    condition_threshold {
      filter             = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"get-imdb-rating-service\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.labels.response_code_class = \"5xx\""
      denominator_filter = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"get-imdb-rating-service\" AND metric.type = \"run.googleapis.com/request_count\""
      comparison         = "COMPARISON_GT"
      threshold_value    = 0.02
      duration           = "300s"
      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_RATE"
        cross_series_reducer = "REDUCE_SUM"
      }
      denominator_aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_RATE"
        cross_series_reducer = "REDUCE_SUM"
      }
    }
  }
}

resource "google_monitoring_alert_policy" "imdb_latency" {
  project               = var.project_id
  display_name          = "MoviesTracker IMDb service latency"
  combiner              = "OR"
  notification_channels = local.notification_channels
  conditions {
    display_name = "p99 latency exceeds two seconds"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"get-imdb-rating-service\" AND metric.type = \"run.googleapis.com/request_latencies\""
      comparison      = "COMPARISON_GT"
      threshold_value = 2000
      duration        = "300s"
      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_PERCENTILE_99"
      }
    }
  }
}

resource "google_monitoring_alert_policy" "frontend_5xx" {
  project               = var.project_id
  display_name          = "MoviesTracker frontend 5xx rate"
  combiner              = "OR"
  notification_channels = local.notification_channels
  conditions {
    display_name = "5xx responses exceed 2 percent"
    condition_threshold {
      filter             = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.frontend.name}\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.labels.response_code_class = \"5xx\""
      denominator_filter = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.frontend.name}\" AND metric.type = \"run.googleapis.com/request_count\""
      comparison         = "COMPARISON_GT"
      threshold_value    = 0.02
      duration           = "300s"
      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_RATE"
        cross_series_reducer = "REDUCE_SUM"
      }
      denominator_aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_RATE"
        cross_series_reducer = "REDUCE_SUM"
      }
    }
  }
}

resource "google_monitoring_alert_policy" "frontend_latency" {
  project               = var.project_id
  display_name          = "MoviesTracker frontend warm latency"
  combiner              = "OR"
  notification_channels = local.notification_channels
  conditions {
    display_name = "p95 latency exceeds two seconds"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.frontend.name}\" AND metric.type = \"run.googleapis.com/request_latencies\""
      comparison      = "COMPARISON_GT"
      threshold_value = 2000
      duration        = "300s"
      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_PERCENTILE_95"
      }
    }
  }
}
