package serveapi

import "regexp"

// mustCompile is a tiny helper so handler files can keep their var blocks tidy.
func mustCompile(s string) *regexp.Regexp { return regexp.MustCompile(s) }
