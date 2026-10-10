package cleaners

func init() {
	// APT package cache: downloaded .deb archives are always re-downloadable,
	// so retention is not applied by default. We delete the files directly
	// (equivalent to `apt-get clean`) so the operation shows up in --dry-run.
	register(&pathCleaner{
		key: "apt-cache", title: "APT package cache", icon: "", // nf-mdi-package
		category: catSystem, mode: modeFiles, detail: "apt archive",
		useRetention: false,
		available:    func() bool { return dirExists("/var/cache/apt/archives") },
		roots: func(_ *Context) []string {
			return []string{
				"/var/cache/apt/archives/*.deb",
				"/var/cache/apt/archives/partial/*.deb",
			}
		},
	})
}
