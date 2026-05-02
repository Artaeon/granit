package agentruntime

import "time"

// nowYMD returns the local-time YYYY-MM-DD for "today". A package-level
// var (not a const) so tests can stub the clock without injecting a
// time-of-day dependency through every public function.
var nowYMD = func() string { return time.Now().Local().Format("2006-01-02") }
