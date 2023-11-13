# Maintenance procedure

This page describes how to update tenet regularly.

## Regular Update

1. Run `make setup`.
2. Update `go.mod`.
3. Update `k8s-version` used in GitHub Actions. They are kind node tags.
4. Update `KUBERNETES_VERSION` in `Makefile.versions`. It is also a kind node tag.
5. Update Go & Ubuntu versions if needed.
6. Check for new software versions using `make version`. You may be prompted to login to github.com.
   ```console
   $ make version
   ```
7. Check `Makefile.versions` and revert some changes that you don't want now.
8. Update software versions using `make maintenance`.
   ```console
   $ make maintenance
   ```
9. Follow [release.md](/docs/release.md) to update software version.
