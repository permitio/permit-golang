package api

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
	if url[0:5] == "https" {
		return "https"
	}
	return "http"
}

func getHostFromUrl(url string) string {
	schema := getSchemaFromUrl(url)
	return url[len(schema)+3:]
}
