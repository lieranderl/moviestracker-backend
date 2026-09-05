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
