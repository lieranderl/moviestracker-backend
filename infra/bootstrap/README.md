# State bootstrap

This one-time stack creates the regional, private, versioned GCS bucket used by
the production OpenTofu backend. Google-managed encryption is enabled by
default; archived state generations expire after 30 days to control storage
cost.

```sh
tofu init
tofu plan -out=bootstrap.tfplan
tofu apply bootstrap.tfplan
```

The stack also creates release-tag-only GitHub OIDC trust and separate backend
and frontend deployers. Trust uses immutable repository IDs and strict semantic
version tags. Both can push to the MoviesTracker Artifact Registry repository;
their exact Cloud Run and service-account permissions are attached in `prod/`.
Neither can change infrastructure, IAM, secrets, databases, monitoring,
schedulers, APIs, or OpenTofu state. Never add application service accounts to
the state bucket.

## CI identities for `infra/prod`

The stack also creates two identities that `.github/workflows/` use, each in
its own Workload Identity pool that trusts only specific workflow files on
`refs/heads/master`:

| Identity | Pool | Trusted workflows | Can |
| --- | --- | --- | --- |
| `movies-infra-planner@` | `moviestracker-github-plan` | `drift.yml`, `infra.yml` | read the project and the state (`roles/viewer`, `roles/iam.securityReviewer`) |
| `movies-infra-applier@` | `moviestracker-github-apply` | `infra.yml` in the `infrastructure` environment | administer every resource type in `prod/`, write the state |

The applier can effectively grant itself anything in the project, so the
`infrastructure` environment's required reviewer is the control that matters.

### One-time setup

Run as a project owner, from this directory:

```sh
tofu plan -out=bootstrap.tfplan    # expect only additions
tofu apply bootstrap.tfplan
```

Next create the `infrastructure` environment, **before** setting any variable
below. GitHub creates a missing environment without protection rules, so a
workflow run in between could apply unattended. Give it a required reviewer,
limit deployments to `master`, and turn off administrator bypass (repository
Settings > Environments). `infra.yml` also refuses to plan while the
environment has no required reviewer.

Then, from the repository root:

```sh
gh variable set GCP_PLAN_WORKLOAD_ID_PROVIDER --body "$(tofu -chdir=infra/bootstrap output -raw github_plan_workload_identity_provider)"
gh variable set GCP_PLAN_SERVICE_ACCOUNT --body "$(tofu -chdir=infra/bootstrap output -raw github_planner_service_account)"
gh variable set GCP_APPLY_WORKLOAD_ID_PROVIDER --body "$(tofu -chdir=infra/bootstrap output -raw github_apply_workload_identity_provider)"
gh variable set GCP_APPLY_SERVICE_ACCOUNT --body "$(tofu -chdir=infra/bootstrap output -raw github_applier_service_account)"
```

Store the production variables as JSON in the `PROD_TFVARS_JSON` secret. The
keys are those of `prod/terraform.tfvars.example`; `images` must still be valid
digests, but releases move the images and `prod/` ignores them:

```sh
gh secret set PROD_TFVARS_JSON < prod.tfvars.json
```
