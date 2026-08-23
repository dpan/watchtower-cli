package cli

import (
	"fmt"
	"net/url"
	"strings"
)

func filterQuery(images, containers []string) url.Values {
	q := url.Values{}
	if len(images) > 0 {
		q.Set("image", joinComma(images))
	}
	if len(containers) > 0 {
		q.Set("container", joinComma(containers))
	}
	return q
}

func nameQuery(names, images []string) url.Values {
	q := url.Values{}
	if len(names) > 0 {
		q.Set("name", joinComma(names))
	}
	if len(images) > 0 {
		q.Set("image", joinComma(images))
	}
	return q
}

func imageQuery(names, ids []string) url.Values {
	q := url.Values{}
	if len(names) > 0 {
		q.Set("name", joinComma(names))
	}
	if len(ids) > 0 {
		q.Set("id", joinComma(ids))
	}
	return q
}

func historyQuery(since, until string, limit int) url.Values {
	q := url.Values{}
	if since != "" {
		q.Set("since", since)
	}
	if until != "" {
		q.Set("until", until)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	return q
}

func joinComma(values []string) string {
	return strings.Join(values, ",")
}
