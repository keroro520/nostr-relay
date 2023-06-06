package nostr_relay

import (
	"github.com/nbd-wtf/go-nostr"
	"strconv"
	"strings"
)

// TODO: Prevent SQL Attacks
// TODO: Prevent SQL Injections

func filtersToSQLCond(filters *nostr.Filters) string {
	conditions := make([]string, len(*filters))
	for i, filter := range *filters {
		conditions[i] = filterToSQLCond(&filter)
	}
	return strings.Join(conditions, " OR ")
}

func filterToSQLCond(filter *nostr.Filter) string {
	conditions := make([]string, 0)

	if len(filter.IDs) > 0 {
		conditions = append(conditions, filterIDsToSQLCond(filter.IDs))
	}
	if len(filter.Authors) > 0 {
		conditions = append(conditions, filterAuthorsToSQLCond(filter.Authors))
	}
	if len(filter.Kinds) > 0 {
		conditions = append(conditions, filterKindsToSQLCond(filter.Kinds))
	}
	if filter.Tags != nil {
		if etag, ok := filter.Tags["#e"]; ok && len(etag) != 0 {
			conditions = append(conditions, filterEtagToSQLCond(etag))
		}
		if ptag, ok := filter.Tags["#p"]; ok && len(ptag) != 0 {
			conditions = append(conditions, filterPtagToSQLCond(ptag))
		}
	}
	if filter.Since != nil {
		conditions = append(conditions, filterSinceToSQLCond(*filter.Since))
	}
	if filter.Until != nil {
		conditions = append(conditions, filterUntilToSQLCond(*filter.Until))
	}
	if filter.Limit != 0 {
		conditions = append(conditions, filterLimitToSQLCond(filter.Limit))
	}

	if len(conditions) == 0 {
		return "true"
	} else {
		return "(" + strings.Join(conditions, " AND ") + ")"
	}
}

func filterIDsToSQLCond(filterIDs []string) string {
	return "id IN (" + strings.Join(quoteStringSlice(filterIDs), ",") + ")"
}

func filterAuthorsToSQLCond(filterAuthors []string) string {
	return "pubkey IN (" + strings.Join(quoteStringSlice(filterAuthors), ",") + ")"
}

func filterKindsToSQLCond(filterKinds []int) string {
	return "kind IN (" + strings.Join(intSliceToStringSlice(filterKinds), ",") + ")"
}

func filterEtagToSQLCond(filterEtag nostr.Tag) string {
	return "etag @> '{" + strings.Join(quoteStringSlice(filterEtag), ",") + "}'"
}

func filterPtagToSQLCond(filterPtag nostr.Tag) string {
	return "ptag @> '{" + strings.Join(quoteStringSlice(filterPtag), ",") + "}'"
}

func filterSinceToSQLCond(filterSince nostr.Timestamp) string {
	return "timestamp >= " + strconv.FormatInt(int64(filterSince), 10)
}

func filterUntilToSQLCond(filterUntil nostr.Timestamp) string {
	return "timestamp <= " + strconv.FormatInt(int64(filterUntil), 10)
}

func filterLimitToSQLCond(filterLimit int) string {
	return "LIMIT " + strconv.Itoa(filterLimit)
}

func intSliceToStringSlice(intSlice []int) []string {
	stringSlice := make([]string, len(intSlice))
	for i, val := range intSlice {
		stringSlice[i] = strconv.Itoa(val)
	}
	return stringSlice
}

func quoteStringSlice(stringSlice []string) []string {
	quotedStringSlice := make([]string, len(stringSlice))
	for i, val := range stringSlice {
		quotedStringSlice[i] = "'" + val + "'"
	}
	return quotedStringSlice
}
