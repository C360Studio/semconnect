# Read-only discovery commands

All commands were run from the semconnect checkout. No workflow was dispatched,
no deployment API was written, and no production NATS data was queried.

## Local context

```console
git remote -v
git branch --show-current
command -v gh kubectl helm nats docker aws gcloud az terraform opentofu
env | cut -d= -f1 | sort
kubectl config get-contexts
kubectl config current-context
docker context ls
gh auth status
```

Only environment-variable names were inspected. The GitHub CLI redacted its
token. NATS context directories, Kubernetes config, cloud config directories,
and SSH Host aliases were checked by existence/name only; credential contents
were not read.

## GitHub semconnect metadata

```console
gh api repos/C360Studio/semconnect
gh api repos/C360Studio/semconnect/environments
gh api repos/C360Studio/semconnect/actions/workflows
gh api 'repos/C360Studio/semconnect/actions/runs?per_page=30'
gh api 'repos/C360Studio/semconnect/deployments?per_page=100'
gh api repos/C360Studio/semconnect/actions/variables
gh api repos/C360Studio/semconnect/actions/secrets
gh api 'repos/C360Studio/semconnect/releases?per_page=100'
gh api 'repos/C360Studio/semconnect/git/trees/main?recursive=1'
gh workflow view conformance --repo C360Studio/semconnect --yaml
```

Every response was reduced with `--jq` to identity/status metadata. Secret
values are not available from the list endpoint and were not requested.

## Organization and adjacent deployment-source checks

```console
gh api 'orgs/C360Studio/repos?type=all&per_page=100' --paginate
gh api 'orgs/C360Studio/packages?package_type=container&per_page=100'
gh api orgs/C360Studio/actions/variables
gh api orgs/C360Studio/actions/secrets
gh api repos/C360Studio/semops
gh api 'repos/C360Studio/semops/git/trees/main?recursive=1'
gh api repos/C360Studio/semops/environments
gh api repos/C360Studio/semops/actions/workflows
gh api 'repos/C360Studio/semops/deployments?per_page=100'
gh api repos/C360Studio/semops/actions/variables
gh api repos/C360Studio/semops/actions/secrets
```

Organization variables/secrets returned HTTP 403 for lack of administrator
permission. Container package listing returned HTTP 403 for lack of
`read:packages`. No privilege escalation or token-scope change was attempted.

## Repository audit

```console
rg --files .github/workflows
rg --files | rg -i 'deploy|infra|ops|k8s|helm|terraform|production|rollback|reseed'
rg -n -i 'production|nats|rollback|reseed|image digest|maintenance window'
rg -n 'ENTITY_STATES|SPATIAL_INDEX|CS_API_OBSERVATIONS|CS_API_ARTIFACTS'
rg -n -i 'ghcr.io|docker.io|quay.io|image:|@sha256:'
```

Disposable conformance output and beta.147/beta.149 signed evidence were not
used as production values.
