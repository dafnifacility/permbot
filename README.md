# Permbot - Automatic Kubernetes Permissions

![permbot-logo](permbot-logo.png)

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
   repository via Kubernetes `update` operations.

### Command-line options

```
Usage of ./permbot:
  -debug
    	Enable debug logging
  -global
    	Also create/display globally scoped resources (ClusterRole/ClusterRoleBinding) (default true)
  -mode string
    	Mode - either yaml or k8s (default "yaml")
  -namespace string
    	Only dump specific namespace - for yaml mode
  -owner string
    	Owner value for Kubernetes label (default "permbot")
  -ref string
    	Version of input repository to include in rule annotations (dafni.ac.uk/permbot-rules-ref)
  -version
    	Exit, only printing Permbot version
```

Note that the `-ref` flag can be used to add a rules "reference" version as an
annotation to create resources, this can be used to track the (e.g) Git SHA of a
commit/tag used to perform a deployment.

Additionally, the `-owner` flag can be used to manipulate a label on created objects,
which could be used to search for objects created by a particular invocation of Permbot.

## Development

This was written by James Hannah in January 2020. Some tasks that still need doing:

- Role Bindings should be automatically emptied/deleted if the project is removed from
  the config file. You can currently remove existing permissions by leaving the project
  in the config file but truncating the list of users to none, but this isn't ideal and
  is a slight security risk(?)
- Slack notifications should be sent on changes (but this would mean detecting when changes
  were made)
- Possibly it'd make sense to have the permissions in LDAP or something, instead of in a
  config file

## Maintenance

The code for this project is fairly simple Golang, and should "just work". There's a
chance that the dependencies may need updating periodically due to Kubernetes API
version changes (although the RBAC API should be fairly stable by now).

## Building PermBot

Permbot can be compiled by simply running `make` from the checked-out repository. This
requires only the Go compiler (written on version 1.15 originally). Alternatively, a
Docker image can be built using the DockerFile.

Permbot is automatically compiled and published as a Docker image by Gitlab CI within DAFNI.
