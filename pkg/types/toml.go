package types

// PermbotConfig is for unmarshalling a TOMl struct into
type PermbotConfig struct {
	Projects []Project `toml:"project" json:"project"`
	Roles    []Role    `toml:"role" json:"role"`
}

// Project defines a single namespace and the applicable roles
type Project struct {
	Namespace string `toml:"namespace" json:"namespace"`
	// GitlabPath  string      `toml:"gitlabPath",json:"gitlabPath"`
	Roles []RoleUsers `toml:"roles" json:"roles"`
}

// RoleUsers links a Role to a set of Users
type RoleUsers struct {
	Role            string   `toml:"role" json:"role"`
	Users           []string `toml:"users" json:"users"`
	ServiceAccounts []string `toml:"serviceAccounts" json:"serviceAccounts"`
}

// Role is a defined Role (or ClusterRole, if global users are specified)
type Role struct {
	Name                  string   `toml:"name" json:"name"`
	Rules                 []Rule   `toml:"rules" json:"rules"`
	GlobalUsers           []string `toml:"globalUsers" json:"globalUsers"`
	GlobalServiceAccounts []string `toml:"globalServiceAccounts" json:"globalServiceAccounts"`
}

// Rule is a specific rule allowed as part of a Role/ClusterRole
type Rule struct {
	APIGroups []string `toml:"apiGroups" json:"apiGroups"`
	Resources []string `toml:"resources" json:"resources"`
	Verbs     []string `toml:"verbs" json:"verbs"`
}
