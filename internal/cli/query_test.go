package cli

import "testing"

func TestQueryBuilders(t *testing.T) {
	q := filterQuery([]string{"a", "b"}, []string{"^web-.*"})
	if q.Get("image") != "a,b" {
		t.Errorf("image: %q", q.Get("image"))
	}
	if q.Get("container") != "^web-.*" {
		t.Errorf("container: %q", q.Get("container"))
	}

	q = nameQuery([]string{"nginx"}, []string{"nginx:latest"})
	if q.Get("name") != "nginx" || q.Get("image") != "nginx:latest" {
		t.Errorf("nameQuery: %v", q)
	}

	q = imageQuery([]string{"nginx:latest"}, []string{"sha256:1111"})
	if q.Get("name") != "nginx:latest" || q.Get("id") != "sha256:1111" {
		t.Errorf("imageQuery: %v", q)
	}

	q = historyQuery("2025-01-20T11:00:00Z", "2025-01-20T12:00:00Z", 50)
	if q.Get("since") != "2025-01-20T11:00:00Z" || q.Get("until") != "2025-01-20T12:00:00Z" || q.Get("limit") != "50" {
		t.Errorf("historyQuery: %v", q)
	}
}
