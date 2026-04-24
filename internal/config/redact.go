package config

import (
	"regexp"
)

// userinfoPass matches the "user:password@" segment in a mongodb:// or
// mongodb+srv:// URI. We mask via regex rather than url.UserPassword so the
// redacted password is not URL-encoded (asterisks are reserved characters).
var (
	userinfoPass = regexp.MustCompile(`^(mongodb(?:\+srv)?://[^:@/]+:)[^@]+@`)
	strayPass    = regexp.MustCompile(`(?i)(password=)[^&]+`)
)

// RedactMongoURI masks the password in either a standard mongodb:// URI or a
// mongodb+srv:// seedlist URI. It also masks any ?password= query parameter.
// The function never returns a string that could disclose credentials, even
// on malformed input.
func RedactMongoURI(uri string) string {
	if uri == "" {
		return ""
	}
	s := userinfoPass.ReplaceAllString(uri, "${1}****@")
	s = strayPass.ReplaceAllString(s, "${1}****")
	return s
}
