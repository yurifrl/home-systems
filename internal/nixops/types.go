package nixops

// NisOps
type SchemaVersion struct {
	Version int
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
	Name  string
	Value string
}

type Nixops struct {
	Deployments   []Deployment
	Resource      []Resource
	ResourceAttr  []ResourceAttr
	SchemaVersion []SchemaVersion
}
