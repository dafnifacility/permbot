package k8s

import (
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/internal/app"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/pkg/types"
)

const (
	roleName  = "permbot-auto-role"
	ownerName = "permbot"
)

// objectAnnotations returns the default annotations to be added to all created objects,
// which contain the version of permbot used to create them.
//
// The option rulesVersion parameter can be used to add an annotation containing the
// version of input config used to generate the rules (if non-empty).
func objectAnnotations(rulesRef string) map[string]string {
	annotations := map[string]string{
		"dafni.ac.uk/permbot-version": app.Version(),
	}
	if rulesRef != "" {
		annotations["dafni.ac.uk/permbot-rules-ref"] = rulesRef
	}
	return annotations
}

// objectLabels returns the default labels to be added to all created objects.
func objectLabels(ownerName string) map[string]string {
	return map[string]string{
		"dafni.ac.uk/permbot-owner": ownerName,
	}
}

// CreateGlobalResources returns the global ClusterRole and ClusterRoleBindings defined by the configuration
func CreateGlobalResources(fromconfig *types.PermbotConfig, rulesRef, owner string) (roles []rbacv1.ClusterRole, rolebindings []rbacv1.ClusterRoleBinding, err error) {
	for i := range fromconfig.Roles {
		cr := fromconfig.Roles[i]
		subjectCount := len(cr.GlobalUsers) + len(cr.GlobalServiceAccounts)
		if subjectCount > 0 {
			// At least one GlobalUsers/GlobalServiceAccounts is listed, so we need to define this as a
			// ClusterRole+ClusterRoleBinding.
			log.WithField("role_name", cr.Name).Debugf("defining as clusterrole+clusterrolebinding due to %d global subjects", subjectCount)
			crole := rbacv1.ClusterRole{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterRole",
					APIVersion: "rbac.authorization.k8s.io",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:        fmt.Sprintf("%s-global-%s", roleName, cr.Name),
					Labels:      objectLabels(owner),
					Annotations: objectAnnotations(rulesRef),
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
					Name:        fmt.Sprintf("%s-global-binding-%s", roleName, cr.Name),
					Labels:      objectLabels(owner),
					Annotations: objectAnnotations(rulesRef),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     crole.Name,
				},
				Subjects: make([]rbacv1.Subject, subjectCount),
			}
			// i is just a runnning count of the number of subjects we've added to subjects so far.
			// it should eventually be the same as subjectCount so we check for that later
			i := 0
			// First add the globalUsers. Note that cru/crsa in the two loops below is the array index (an int)
			for cru := range cr.GlobalUsers {
				crb.Subjects[i] = rbacv1.Subject{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "User",
					Name:     cr.GlobalUsers[cru],
				}
				i++
			}
			// Then add the global service accounts, these have to be prefixed with the namespace
			for crsa := range cr.GlobalServiceAccounts {
				nsnparts := strings.SplitN(cr.GlobalServiceAccounts[crsa], ":", 2)
				if len(nsnparts) == 1 {
					nsnparts = append(nsnparts, nsnparts[0])
					nsnparts[0] = "default"
				} else if nsnparts[1] == "" {
					nsnparts[1] = nsnparts[0]
					nsnparts[0] = "default"
				}
				crb.Subjects[i] = rbacv1.Subject{
					APIGroup:  "rbac.authorization.k8s.io",
					Kind:      "ServiceAccount",
					Name:      nsnparts[1],
					Namespace: nsnparts[0],
				}
				i++
			}
			// Catch the unlikely event that we didn't fully complete the set of subjects
			if i != subjectCount {
				log.WithFields(log.Fields{
					"subjects-count": subjectCount,
					"subjects-added": i,
					"global-role":    roleName,
				}).Fatal("subject count mismatch when adding subjects to global role")
			}
			rolebindings = append(rolebindings, crb)
		}
	}
	return
}

// CreateResourcesForNamespace creates a set of Roles and a set of RoleBindings for the
// specified namespace, based on the
func CreateResourcesForNamespace(fromconfig *types.PermbotConfig, ns, rulesRef, ownerName string) (roles []rbacv1.Role, rolebindings []rbacv1.RoleBinding, err error) {
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
						Name:        fmt.Sprintf("%s-%s", roleName, rl.Name),
						Namespace:   fromconfig.Projects[pr].Namespace,
						Labels:      objectLabels(ownerName),
						Annotations: objectAnnotations(rulesRef),
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
						Name:        fmt.Sprintf("%s-binding-%s", roleName, rl.Name),
						Namespace:   fromconfig.Projects[pr].Namespace,
						Labels:      objectLabels(ownerName),
						Annotations: objectAnnotations(rulesRef),
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "Role",
						Name:     role.Name,
					},
					Subjects: make([]rbacv1.Subject, len(fromconfig.Projects[pr].Roles[prr].Users)+len(fromconfig.Projects[pr].Roles[prr].ServiceAccounts)),
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
				// Need to offset the set-index by this count to not whallop Users
				cusers := len(fromconfig.Projects[pr].Roles[prr].Users)
				for rrsa := range fromconfig.Projects[pr].Roles[prr].ServiceAccounts {
					saname := fromconfig.Projects[pr].Roles[prr].ServiceAccounts[rrsa]
					sans := fromconfig.Projects[pr].Namespace
					if strings.Contains(saname, ":") {
						ps := strings.SplitN(saname, ":", 2)
						sans = ps[0]
						saname = ps[1]
					}
					rolebinding.Subjects[cusers+rrsa] = rbacv1.Subject{
						APIGroup:  "",
						Kind:      "ServiceAccount",
						Name:      saname,
						Namespace: sans,
					}
				}
				rolebindings = append(rolebindings, rolebinding)
			}
		}
	}
	return
}
