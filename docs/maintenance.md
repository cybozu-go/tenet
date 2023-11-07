# Maintenance procedure

This page describes how to update tenet regularly.

## Regular Update

1. Update `go.mod`.
2. Update `k8s-version` used in GitHub Actions. They are kind node tags.
3. Update `KUBERNETES_VERSION` in `Makefile.versions`.
4. Update Go & Ubuntu versions if needed.
5. Check for new software versions using `make version`. You may be prompted to login to github.com.
   ```console
   $ make version
   ```
6. Check `Makefile.versions` and revert some changes that you don't want now.
7. Update software versions using `make maintenance`.
   ```console
   $ make maintenance
   ```
8. Follow [release.md](/docs/release.md) to update software version.
