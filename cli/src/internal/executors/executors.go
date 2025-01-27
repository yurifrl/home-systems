package executors

type Executor struct {
	currentWorkdir string
	dockerWorkdir  string
	image          string
}

func NewExecutor() *Executor {
	return &Executor{
		image:          "ghcr.io/yurifrl/home-systems",
		currentWorkdir: ".",
		dockerWorkdir:  "/src",
	}
}
