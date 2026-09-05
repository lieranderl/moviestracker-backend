# Production infrastructure

`bootstrap/` creates the private, regional, versioned state bucket plus the
release-tag-only GitHub OIDC trust. `prod/` manages the existing Firestore
database, IMDb bucket, Artifact Registry repository, API Gateway, one service,
four jobs/schedules, least-privilege runtime identities, and alerts.

## First adoption

1. Apply `bootstrap/` once as a platform administrator.
2. Copy `terraform.tfvars.example` to an untracked tfvars file and replace every
   image with an Artifact Registry digest and every Rutor URL placeholder.
3. Run `tofu -chdir=infra/prod init`, then create and review a saved plan. The
   declarative import blocks adopt active resources before changing them.
4. To make backend legacy deletion explicit, first import with
   `-var='retire_backend_legacy_resources=false'`:

   ```sh
   tofu -chdir=infra/prod import -var='retire_backend_legacy_resources=false' 'google_cloud_run_v2_service.retired_torrent[0]' 'projects/moviestracker-f07e2/locations/europe-west1/services/get-torrents-for-movie-service'
   tofu -chdir=infra/prod import -var='retire_backend_legacy_resources=false' 'google_secret_manager_secret.retired_kinozal[0]' 'projects/moviestracker-f07e2/secrets/KZ_PASSWORD'
   ```

5. Plan with the default `retire_backend_legacy_resources=true`. The reviewed
   plan must show the explicit torrent/Kinozal deletions, removal of the old
   gateway route, and no replacement database. MongoDB and Firebase secrets
   stay unmanaged until the Firestore/JWT frontend has been deployed and
   verified.

6. After frontend cutover, import its legacy secrets with
   `retire_frontend_legacy_resources=false`, then review a plan with
   `retire_frontend_legacy_resources=true` before deleting them.

No 14-day parallel stack is retained. Release safety comes from protected plan
approval, immutable digests, fresh-data jobs, smoke tests, and immediate Cloud
Run traffic rollback if the release fails.

## Cost controls

- Cloud Run service min instances: 0; max: 2
- Jobs: one task, one retry, 512 MiB
- IMDb object and state versions expire automatically
- Artifact Registry keeps ten recent images and removes images older than 30 days
- no development cloud environment, database credentials, GCS FUSE, or MongoDB
