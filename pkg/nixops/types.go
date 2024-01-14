package nixops

type SchemaVersion struct {
	Version int
}

type DeploymentAttr struct {
	Deployment string
	Name       string
	Value      string
}

type Resource struct {
	ID         int
	Deployment string
	Name       string
	Type       string
}

type ResourceAttr struct {
	Machine int
	Name    string
	Value   string
}

type Deployment struct {
	UUID  string
	Attrs map[string]string // Using a map to store attributes
}

type Nixops struct {
	Deployments   []Deployment
	Resource      []Resource
	ResourceAttr  []ResourceAttr
	SchemaVersion []SchemaVersion
}
