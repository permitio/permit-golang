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
