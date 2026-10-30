locals {
  catalog_jobs = {
    latesttorrentsmovies = {
      schedule = "0 23,8,13,20 * * *"
    }
    hdr10movies = {
      schedule = "0 2 * * *"
    }
    dvmovies = {
      schedule = "0 1 * * *"
    }
  }

  scheduler_jobs = merge(
    { for collection, job in local.catalog_jobs : "getmovies-${collection}-job-scheduler-trigger" => {
      job_name = "getmovies-${collection}-job"
      schedule = job.schedule
    } },
    {
      "downloadimdbdata-job-scheduler-trigger" = {
        job_name = "downloadimdbdata-job"
        schedule = "0 2 * * *"
        # A fixed 24h cadence keeps the freshness window valid across DST.
        time_zone = "Etc/UTC"
      }
    }
  )
}
