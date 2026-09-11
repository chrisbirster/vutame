# Branching and releases

Vutame uses a permanent `dev` integration branch and a production-only `main` branch.

```text
feature/* ──PR──▶ dev ──release PR──▶ main
                    │                  │
                    └─ continuous CI   └─ tag vX.Y.Z
```

## Feature work

1. Start from the latest `dev`.
2. Create a focused branch such as `feat/m1-auth`, `feat/profile-themes`, `fix/profile-404`, or `docs/atproto`.
3. Open a pull request into `dev`.
4. Require CI to pass before merge.
5. Prefer squash merges so each feature lands on `dev` as one coherent change.

Direct feature work must not target `main`.

## Releases

When `dev` is release-ready:

1. Freeze the intended release scope on `dev` and ensure CI is green.
2. Update version/changelog/release notes in a release-prep change on `dev` if needed.
3. Open a pull request from `dev` to `main` titled `Release vX.Y.Z`.
4. Verify the release PR against the exact `dev` head.
5. Merge `dev` into `main` without unrelated changes.
6. Create annotated tag `vX.Y.Z` on the resulting `main` release commit.
7. Deploy/build release artifacts from that tag.
8. Keep `dev` moving forward; never develop directly on the release tag.

Pre-releases use tags such as `v0.1.0-rc.1` when a deployable candidate is needed before the stable tag.

## Branch protection target

Both permanent branches should eventually require pull requests and CI. `main` should additionally reject direct pushes and only receive release promotions from `dev` except for emergency hotfixes. A hotfix starts from `main`, is merged to `main`, tagged, and immediately back-merged/cherry-picked to `dev` so history does not diverge.
