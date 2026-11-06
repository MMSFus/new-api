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

CI (`.github/workflows/fork-ci.yml`) replaces the upstream `ci.yml`, which is kept unmodified so upstream syncs do not conflict, and is disabled in the fork's Actions settings.

- Branch push without an open pull request: incremental check of what changed since the merge base with `main` (backend when Go code changed, frontend tests related to changed `web/` files).
- Pull request into `main`: full check (backend and 3 frontend test shards in parallel) when it is opened and on every later push to its branch.
- Merge to `main`: no re-check, because branch protection requires the branch to be up to date and `main` therefore equals the checked pull request head. After 30 seconds the image is built; a newer merge cancels the older run, so only the latest merge is published. Images are built only from `main`.

Merges to `main` publish two Docker Hub tags (credentials come from the `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` repository secrets):

```text
docker.io/mmsfus/new-api:main
docker.io/mmsfus/new-api:sha-<short-commit>
```

Images published before the switch remain on `ghcr.io/mmsfus/new-api`.

## Server deployment

Production (`/opt/services/new-api/compose.yml`) runs `docker.io/mmsfus/new-api:main` with the label `com.centurylinklabs.watchtower.enable: "true"`. The Watchtower instance on the server checks labelled containers every day at 03:20 (server time) and recreates `new-api` when `main` has a new digest, so a merge to `main` reaches production that night.

Watchtower does not back up the database. Before merging a change with a migration, back up PostgreSQL yourself, or pin the image to the immutable `sha-*` tag (or the digest shown in the workflow summary) and deploy it manually as below. To pin or roll back, set `image:` to `docker.io/mmsfus/new-api:sha-<short-commit>` and remove the Watchtower label.

To deploy immediately instead of waiting for Watchtower, back up the database, then run:

```bash
cd /opt/services/new-api
docker compose config --quiet
docker compose pull new-api
docker compose up -d --wait new-api
docker compose ps
docker compose logs --tail=100 new-api
```

Do not edit files inside the running container; container-local changes are discarded when it is recreated.
