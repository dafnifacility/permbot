# Permbot Changelog

## v1.2.0

This is a feature release of Permbot.

New Features:
- `role` objects can now contain a new field `globalServiceAccounts`, which will cause a ClusterRole to be created containing the ServiceAccounts specified. ServiceAccounts should be specified as `namespace:serviceaccountname`.
- Some tests have been added, and are now run automatically in the CI
  
## v1.1.1

This is the initial public release of Permbot, which is now mirrored from the private DAFNI
Gitlab instance to the public DAFNI Github.

- Properly annotate/label created rules/bindings with Permbot info including version of
  tool plus input config version.
- Add support for building with Github Actions
- Add CODEOWNERS & LICENSE
- Add usage docs

## v1.1.0

- Add logo
- Add documentation
- Add support for ServiceAccount permissions

## v1.0.1

- Track software version in code properly (add version flag)
- Cleanup build process using Makefile

## v1.0.1

- Add Examples
- Add YAML dump output

## v1.0.0

This is the initial internal release of "Permbot", a tool for managing Kubernetes Roles,
ClusterRoles and associated bindings with Users via continuous integration.
