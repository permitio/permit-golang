package api

import "strings"

const DefaultPerPageLimit = 100

func isPaginationInLimit(page int32, perPage int32, perPageLimit int32) bool {
	return perPage <= perPageLimit && perPage > 0 && page > 0
}

func listToString(list []string) string {
	var str string
	for _, s := range list {
		str += s + ","
	}
	return str
}

func getSchemaFromUrl(url string) string {
	if strings.HasPrefix(url, "https") {
		return "https"
	}
	return "http"
}

// getHostFromUrl returns url without its first len(schema+"://") bytes; it does not check that
// prefix. A url shorter than that has no host, so it returns "" and requests fail with "no Host in
// request URL" instead of NewPermit panicking.
func getHostFromUrl(url string) string {
	hostStart := len(getSchemaFromUrl(url)) + len("://")
	if len(url) < hostStart {
		return ""
	}
	return url[hostStart:]
}
