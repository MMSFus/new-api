# Fork maintenance and deployment

This repository is a customization fork of `QuantumNous/new-api`.

## Branch workflow

- Keep `main` deployable and update it only through pull requests.
- Create work on `feat/*`, `fix/*`, or `chore/*` branches.
- Wait for the protected CI checks before merging.
- Do not commit deployment secrets, server `.env` files, database dumps, or registry tokens.

## Sync upstream

```bash
git fetch upstream
git switch main && git pull --ff-only origin main
git switch -c chore/sync-upstream-YYYYMMDD
git merge --no-ff upstream/main
git push -u origin chore/sync-upstream-YYYYMMDD
```

Resolve customization conflicts in the merge commit, then open a pull request into `main`.

Merge sync pull requests with **Create a merge commit**. Squash or rebase merges rewrite the upstream commits, so the next sync no longer sees them as merged and repeats the same conflicts. Do not rebase the sync branch onto `upstream/main` either, because that rewrites fork commits already on `main`. Feature pull requests may still be squash-merged.

## Image publishing

Merges to `main` run `.github/workflows/publish-custom-image.yml` and publish two GHCR tags:

```text
ghcr.io/mmsfus/new-api:main
ghcr.io/mmsfus/new-api:sha-<short-commit>
```

Use the immutable `sha-*` tag, or preferably the digest shown in the workflow summary, for production deployments. The `main` tag is intended for inspection and testing.

## Server deployment

Before deployment, back up the PostgreSQL database and review migration notes. Then update `/opt/services/new-api/compose.yml` to the selected image and run:

```bash
cd /opt/services/new-api
docker compose config --quiet
docker compose pull new-api
docker compose up -d --wait new-api
docker compose ps
docker compose logs --tail=100 new-api
```

Do not edit files inside the running container; container-local changes are discarded when it is recreated.
