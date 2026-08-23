package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dpan/watchtower-cli/internal/client"
)

func writeBody(w io.Writer, body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if !bytes.HasPrefix(trimmed, []byte("{")) && !bytes.HasPrefix(trimmed, []byte("[")) {
		fmt.Fprintln(w, string(body))
		return nil
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		fmt.Fprintln(w, string(body))
		return nil
	}
	fmt.Fprintln(w, out.String())
	return nil
}

func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeBody(w, b)
}

func httpError(res *client.Result) error {
	msg := strings.TrimSpace(string(res.Body))
	if msg == "" {
		msg = http.StatusText(res.Status)
	}
	if ra := res.Header.Get("Retry-After"); ra != "" {
		return fmt.Errorf("unexpected status %d: %s (retry after %s)", res.Status, msg, ra)
	}
	return fmt.Errorf("unexpected status %d: %s", res.Status, msg)
}
