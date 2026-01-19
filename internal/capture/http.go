package capture

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func newRequestWithContext(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	return req, err
}
