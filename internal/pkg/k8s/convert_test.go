package k8s

import (
	"fmt"
	"reflect"
	"testing"

	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/pkg/types"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateGlobalResources(t *testing.T) {
	type args struct {
		fromconfig *types.PermbotConfig
		rulesRef   string
		owner      string
	}
	tests := []struct {
		name             string
		args             args
		wantRoles        []rbacv1.ClusterRole
		wantRolebindings []rbacv1.ClusterRoleBinding
		wantErr          bool
	}{
		{
			name: "basic-global-service-account",
			args: args{
				fromconfig: &types.PermbotConfig{
					Roles: []types.Role{
						{
							Name:                  "foo",
							GlobalServiceAccounts: []string{"x:foo"},
						},
					},
				},
				owner:    "xyzzy",
				rulesRef: "xxx",
			},
			wantRoles: []rbacv1.ClusterRole{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterRole",
						APIVersion: "rbac.authorization.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-global-%s", roleName, "foo"),
						Labels:      objectLabels("xyzzy"),
						Annotations: objectAnnotations("xxx"),
					},
					Rules: make([]rbacv1.PolicyRule, 0),
				},
			},
			wantRolebindings: []rbacv1.ClusterRoleBinding{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterRoleBinding",
						APIVersion: "rbac.authorization.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-auto-role-global-binding-%s", "permbot", "foo"),
						Labels:      objectLabels("xyzzy"),
						Annotations: objectAnnotations("xxx"),
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     "permbot-auto-role-global-foo",
					},
					Subjects: []rbacv1.Subject{
						{
							APIGroup:  "",
							Kind:      "ServiceAccount",
							Name:      "foo",
							Namespace: "x",
						},
					},
				},
			},
		},
		{
			name: "basic-global-user",
			args: args{
				fromconfig: &types.PermbotConfig{
					Roles: []types.Role{
						{
							Name:        "foo",
							GlobalUsers: []string{"CN=x,DC=example,DC=com"},
						},
					},
				},
				owner:    "xyzzy",
				rulesRef: "xxx",
			},
			wantRoles: []rbacv1.ClusterRole{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterRole",
						APIVersion: "rbac.authorization.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-global-%s", roleName, "foo"),
						Labels:      objectLabels("xyzzy"),
						Annotations: objectAnnotations("xxx"),
					},
					Rules: make([]rbacv1.PolicyRule, 0),
				},
			},
			wantRolebindings: []rbacv1.ClusterRoleBinding{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterRoleBinding",
						APIVersion: "rbac.authorization.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-auto-role-global-binding-%s", "permbot", "foo"),
						Labels:      objectLabels("xyzzy"),
						Annotations: objectAnnotations("xxx"),
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     "permbot-auto-role-global-foo",
					},
					Subjects: []rbacv1.Subject{
						{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "User",
							Name:     "CN=x,DC=example,DC=com",
						},
					},
				},
			},
		},
		{
			name: "complex-global-users-and-service-accounts",
			args: args{
				fromconfig: &types.PermbotConfig{
					Roles: []types.Role{
						{
							Name:        "foo",
							GlobalUsers: []string{"CN=x,DC=example,DC=com"},
							// Missing colon means default namespace
							GlobalServiceAccounts: []string{"whatever"},
						},
					},
				},
				owner:    "xyzzy",
				rulesRef: "xxx",
			},
			wantRoles: []rbacv1.ClusterRole{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterRole",
						APIVersion: "rbac.authorization.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-global-%s", roleName, "foo"),
						Labels:      objectLabels("xyzzy"),
						Annotations: objectAnnotations("xxx"),
					},
					Rules: make([]rbacv1.PolicyRule, 0),
				},
			},
			wantRolebindings: []rbacv1.ClusterRoleBinding{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterRoleBinding",
						APIVersion: "rbac.authorization.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-auto-role-global-binding-%s", "permbot", "foo"),
						Labels:      objectLabels("xyzzy"),
						Annotations: objectAnnotations("xxx"),
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     "permbot-auto-role-global-foo",
					},
					Subjects: []rbacv1.Subject{
						{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "User",
							Name:     "CN=x,DC=example,DC=com",
						},
						{
							APIGroup:  "",
							Kind:      "ServiceAccount",
							Namespace: "default",
							Name:      "whatever",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRoles, gotRolebindings, err := CreateGlobalResources(tt.args.fromconfig, tt.args.rulesRef, tt.args.owner)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateGlobalResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotRoles, tt.wantRoles) {
				t.Errorf("CreateGlobalResources() gotRoles = %v, want %v", gotRoles, tt.wantRoles)
			}
			if !reflect.DeepEqual(gotRolebindings, tt.wantRolebindings) {
				t.Errorf("CreateGlobalResources() gotRolebindings = %v, want %v", gotRolebindings, tt.wantRolebindings)
			}
		})
	}
}
