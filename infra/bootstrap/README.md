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

The stack also creates the release-tag-only GitHub OIDC trust and a deployer
with broad read access but write access limited to Artifact Registry and Cloud
Run. Never add application service accounts to the state bucket.
