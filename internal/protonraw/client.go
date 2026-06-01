// Package protonraw implements Proton API endpoints not exposed by
// go-proton-api: custom-domain CRUD and address creation.
//
// Endpoint paths and payload shapes are sourced from
// https://github.com/ProtonMail/WebClients (read-only reference). Each method
// links to its WebClients counterpart in a comment.
package protonraw

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
)

// Doer is implemented by *session.rawClient. We don't import session to avoid
// a cycle; the interface is just enough to make HTTP calls.
type Doer interface {
	R() *resty.Request
}

// do issues the request built by call against d, then decodes the response
// into out (pass nil to discard the body). The transport error and the decode
// error are both wrapped with label, so each endpoint names its operation
// exactly once instead of repeating the same fmt.Errorf on both paths.
func do(ctx context.Context, d Doer, label string, out any,
	call func(*resty.Request) (*resty.Response, error)) error {
	resp, err := call(d.R().SetContext(ctx))
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if err := decode(resp, out); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func decode(resp *resty.Response, out any) error {
	if resp.IsError() {
		// Return a typed proterr.HTTPError so proterr.Map can route by HTTP
		// status (e.g. 404 -> proton/not_found). A plain fmt.Errorf would
		// lose the status code and bucket every error as proton/upstream.
		return &proterr.HTTPError{
			Status:  resp.StatusCode(),
			Headers: resp.Header(),
			Body:    resp.String(),
		}
	}
	var env struct {
		Code  int    `json:"Code"`
		Error string `json:"Error"`
	}
	if err := json.Unmarshal(resp.Body(), &env); err == nil {
		if env.Error != "" {
			return fmt.Errorf("proton api: %s (code %d)", env.Error, env.Code)
		}
		// Proton's success code is 1000. Non-zero non-1000 codes signal a
		// business error that the API didn't accompany with a string Error.
		if env.Code != 0 && env.Code != 1000 {
			return fmt.Errorf("proton api: unexpected code %d", env.Code)
		}
	}
	if out != nil {
		if err := json.Unmarshal(resp.Body(), out); err != nil {
			return fmt.Errorf("decode body: %w", err)
		}
	}
	return nil
}
