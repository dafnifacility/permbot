# Permbot - Automatic Kubernetes Permissions

This is a tool for applying Kubernetes permissions defined in a Git repository,
automatically.

## Rationale

DAFNI developers currently have global "view" permissions on the entire Kubernetes
cluster, which is good and bad. In a good way, developers are allowed to view and debug
(to some extent, in a readonly way) their own pods. In a bad way, they are allowed to
view secrets or potentially elevate their own access using the information they're able
to acquire.

The main problem with the current situation, is that developers often require more than
just "view" permissions to be able to debug their Pods, specifically they often require
the ability to `kubectl exec` into a Pod to be able to view what it's actually doing.
Right now this is done by manually creating `Role` and `RoleBinding` objects in specific
namespaces, but this is not maintainable.

## Permbot Usage

Permbot works as follows:

1. Developer works on a specific Gitlab project which is deployed via AutoKube
2. Developer wants access to do `kubectl execute` in the pods of their project
3. Developer creates a merge request in the perms repo to add themself to their own
   namespace
4. System administrator merges the request (or not).
5. Permbot automatically creates/revokes Roles/Rolebindings to match the state of the
   repository
