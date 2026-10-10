package cleaners

// dockerCleaner reclaims Docker build cache and dangling images. A full system
// prune (unused images/networks) is opt-in via config because it is more
// aggressive. Named volumes are never pruned.
type dockerCleaner struct{}

func init() { register(&dockerCleaner{}) }

func (d *dockerCleaner) Key() string      { return "docker" }
func (d *dockerCleaner) Title() string    { return "Docker build cache & dangling images" }
func (d *dockerCleaner) Icon() string     { return "" } // nf-linux-docker
func (d *dockerCleaner) Category() string { return catContainers }

func (d *dockerCleaner) Available() bool { return hasCmd("docker") }

func (d *dockerCleaner) Plan(ctx *Context) (*Plan, error) {
	p := &Plan{Cleaner: d.Key(), Title: d.Title(), Icon: d.Icon()}
	if !d.Available() {
		p.Unavailable = true
		p.Note = "docker not installed"
		return p, nil
	}
	// By default, prune only the builder cache and dangling images — safe and
	// re-buildable. Enable [docker] prune/prune_all in the config for more.
	p.Commands = append(p.Commands, Command{
		Label:   "prune docker builder cache",
		Argv:    []string{"docker", "builder", "prune", "-f"},
		Reclaim: -1,
	})
	if ctx.Config.DockerPrune || ctx.Config.DockerPruneAll {
		args := []string{"system", "prune", "-f"}
		label := "prune dangling images/containers/networks"
		if ctx.Config.DockerPruneAll {
			args = append(args, "-a")
			label = "prune ALL unused images/containers/networks"
		}
		p.Commands = append(p.Commands, Command{Label: label, Argv: append([]string{"docker"}, args...), Reclaim: -1})
	} else {
		p.Note = "set [docker] prune=true for a fuller system prune (volumes are never touched)"
	}
	return p, nil
}
