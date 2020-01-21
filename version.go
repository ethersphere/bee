package bee

var (
	version = "v0.1.0" // manually set semantic version number
	commit  string     // automatically set git commit hash

	Version = func() string {
		if commit != "" {
			return version + "-" + commit
		}
		return version
	}()
)
