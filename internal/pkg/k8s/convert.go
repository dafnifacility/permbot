package k8s

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/pkg/types"
)

const (
	roleName  = "permbot-auto-role"
	ownerName = "permbot"
)

// CreateGlobalResources returns the global ClusterRole and ClusterRoleBindings defined by the configuration
func CreateGlobalResources(fromconfig *types.PermbotConfig) (roles []rbacv1.ClusterRole, rolebindings []rbacv1.ClusterRoleBinding, err error) {
	for i := range fromconfig.Roles {
		cr := fromconfig.Roles[i]
		if len(cr.GlobalUsers) > 0 {
			// At least one GlobalUsers is listed, so we need to define this as a
			// ClusterRole+ClusterRoleBinding.
			log.WithField("role_name", cr.Name).Debugf("defining as clusterrole+clusterrolebinding due to %d globalusers", len(cr.GlobalUsers))
			crole := rbacv1.ClusterRole{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterRole",
					APIVersion: "rbac.authorization.k8s.io",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-global-%s", roleName, cr.Name),
					Labels: map[string]string{
						"owner": ownerName,
					},
				},
				Rules: make([]rbacv1.PolicyRule, len(cr.Rules)),
			}
			for crr := range cr.Rules {
				rule := cr.Rules[crr]
				crole.Rules[crr] = rbacv1.PolicyRule{
					APIGroups: rule.APIGroups,
					Verbs:     rule.Verbs,
					Resources: rule.Resources,
				}
			}
			roles = append(roles, crole)
			// Next the CRB
			crb := rbacv1.ClusterRoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterRoleBinding",
					APIVersion: "rbac.authorization.k8s.io",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-global-binding-%s", roleName, cr.Name),
					Labels: map[string]string{
						"owner": ownerName,
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     crole.Name,
				},
				Subjects: make([]rbacv1.Subject, len(cr.GlobalUsers)),
			}
			for cru := range cr.GlobalUsers {
				crb.Subjects[cru] = rbacv1.Subject{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "User",
					Name:     cr.GlobalUsers[cru],
				}
			}
			rolebindings = append(rolebindings, crb)
		}
	}
	return
}

// CreateResourcesForNamespace creates a set of Roles and a set of RoleBindings for the
// specified namespace, based on the
func CreateResourcesForNamespace(fromconfig *types.PermbotConfig, ns string) (roles []rbacv1.Role, rolebindings []rbacv1.RoleBinding, err error) {
	var project *types.Project
	// First we need to find the applicable project
	for i := range fromconfig.Projects {
		if fromconfig.Projects[i].Namespace == ns {
			log.WithField("project", fromconfig.Projects[i].Namespace).Debug("selected single project via ns")
			project = &fromconfig.Projects[i]
		} else {
			log.WithFields(log.Fields{
				"configProject":    fromconfig.Projects[i].Namespace,
				"desiredNamespace": ns,
			}).Debug("skipping project because not a match for target")
		}
	}
	if project == nil {
		// No project found
		err = os.ErrNotExist
		return
	}
	// Next we need to decide what roles are required, this depends on how/if any
	// roleusers define users of roles in the specified namespace
	for ri := range fromconfig.Roles {
		rl := fromconfig.Roles[ri]
		for pr := range fromconfig.Projects {
			if len(fromconfig.Projects[pr].Roles) == 0 {
				// Skip to the next if the project has no roles defined
				log.WithFields(log.Fields{
					"namespace":   fromconfig.Projects[pr].Namespace,
					"currentRole": rl.Name,
				}).Debugf("skipping project because it has no roles")
				continue
			}
			for prr := range fromconfig.Projects[pr].Roles {
				if fromconfig.Projects[pr].Roles[prr].Role != rl.Name {
					log.WithFields(log.Fields{
						"namespace":   fromconfig.Projects[pr].Namespace,
						"role":        fromconfig.Projects[pr].Roles[prr].Role,
						"currentRole": rl.Name,
					}).Debugf("skipping project role because not a match to current role")
					// Skip to the next project role users, because this one doesn't match
					continue
				}
				if fromconfig.Projects[pr].Namespace != project.Namespace {
					log.WithFields(log.Fields{
						"project":   fromconfig.Projects[pr].Namespace,
						"desiredNs": project.Namespace,
					}).Debug("skipping project roles as not a match")
					continue
				}
				// NOTE: code below disabled because we may want to truncate a rolebinding to
				// zero users
				// if len(fromconfig.Projects[pr].Roles[prr].Users) == 0 {
				// 	// Role matches but no users
				// 	log.WithFields(log.Fields{
				// 		"namespace":   fromconfig.Projects[pr].Namespace,
				// 		"role":        fromconfig.Projects[pr].Roles[prr].Role,
				// 		"currentRole": rl.Name,
				// 	}).Debugf("skipping project role because no users defined for it")
				// 	continue
				// }

				// The project defines this role so we define the resource for the namespace
				role := rbacv1.Role{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Role",
						APIVersion: "rbac.authorization.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-%s", roleName, rl.Name),
						Namespace: fromconfig.Projects[pr].Namespace,
						Labels: map[string]string{
							"owner": ownerName,
						},
					},
					Rules: make([]rbacv1.PolicyRule, len(rl.Rules)),
				}
				for rrule := range rl.Rules {
					role.Rules[rrule] = rbacv1.PolicyRule{
						Verbs:     rl.Rules[rrule].Verbs,
						APIGroups: rl.Rules[rrule].APIGroups,
						Resources: rl.Rules[rrule].Resources,
					}
				}
				roles = append(roles, role)
				// Next, the rolebinding
				rolebinding := rbacv1.RoleBinding{
					TypeMeta: metav1.TypeMeta{
						Kind:       "RoleBinding",
						APIVersion: "rbac.authorization.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-binding-%s", roleName, rl.Name),
						Namespace: fromconfig.Projects[pr].Namespace,
						Labels: map[string]string{
							"owner": ownerName,
						},
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "Role",
						Name:     role.Name,
					},
					Subjects: make([]rbacv1.Subject, len(fromconfig.Projects[pr].Roles[prr].Users)),
				}
				// NOTE: if the config previously had rolebinding users for this project, but
				// now doesn't (but is still in the file), they will be removed
				for rru := range fromconfig.Projects[pr].Roles[prr].Users {
					rolebinding.Subjects[rru] = rbacv1.Subject{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "User",
						Name:     fromconfig.Projects[pr].Roles[prr].Users[rru],
					}
				}
				rolebindings = append(rolebindings, rolebinding)
			}
		}
	}
	return
}
