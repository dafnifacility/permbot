package types

type PermbotConfig struct {
	Projects []Project `toml:"project" json:"project"`
	Roles    []Role    `toml:"role" json:"role"`
}

func (pbc *PermbotConfig) CheckReferences() error {
	// TODO: This should check whether roles referenced in projects are defined in the
	// config also
	return nil
}

type Project struct {
	Namespace string `toml:"namespace" json:"namespace"`
	// GitlabPath  string      `toml:"gitlabPath",json:"gitlabPath"`
	Roles []RoleUsers `toml:"roles" json:"role"`
}

type RoleUsers struct {
	Role  string   `toml:"role" json:"role"`
	Users []string `toml:"users" json:"users"`
}

type Role struct {
	Name  string `toml:"name" json:"name"`
	Rules []Rule `toml:"rules" json:"rules"`
}

type Rule struct {
	APIGroups []string `toml:"apiGroups" json:"apiGroups"`
	Resources []string `toml:"resources" json:"resources"`
	Verbs     []string `toml:"verbs" json:"verbs"`
}
