package gcloud

// Properties holds the gcloud config properties gcloud-ctx cares about, read
// from (or written to) a named configuration's INI file. Empty fields mean
// "not set" both when reading and when passed to Store.Create.
type Properties struct {
	Account                   string // core/account
	Project                   string // core/project
	Region                    string // compute/region
	Zone                      string // compute/zone
	ImpersonateServiceAccount string // auth/impersonate_service_account
}

// Config is a named configuration together with its parsed properties.
type Config struct {
	Name string
	Properties

	// ParseErr is non-nil when Store.List could read the configuration's
	// directory entry but failed to parse its backing INI file. Properties
	// is the zero value in that case; callers (the CLI) should render
	// ParseErr instead of the (empty) properties, rather than dropping the
	// entry from the listing entirely.
	ParseErr error
}
