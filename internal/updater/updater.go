package updater

type Status struct {
	Enabled bool   `json:"enabled"`
	Version string `json:"version"`
}

func CurrentStatus(version string) Status {
	return Status{Enabled: false, Version: version}
}
