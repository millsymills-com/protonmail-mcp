package testvcr

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
)

var opaqueIDSegment = regexp.MustCompile(`^[A-Za-z0-9_\-]{8,}$`)

// BodyAwareMatcher matches an incoming request against a recorded interaction.
// Order: method -> normalized path (ID-tolerant) -> sorted-query -> canonical JSON body.
func BodyAwareMatcher(r *http.Request, i cassette.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(r.Method, i.Method) {
		return false
	}
	rURL, err := url.Parse(r.URL.String())
	if err != nil {
		return false
	}
	iURL, err := url.Parse(i.URL)
	if err != nil {
		return false
	}
	if !pathsMatch(rURL.Path, iURL.Path) {
		return false
	}
	if !queriesMatch(rURL.Query(), iURL.Query()) {
		return false
	}
	body, err := readRequestBody(r)
	if err != nil {
		return false
	}
	return bodiesMatch(body, i.Body)
}

func pathsMatch(a, b string) bool {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	if len(as) != len(bs) {
		return false
	}
	for n := range as {
		if as[n] == bs[n] {
			continue
		}
		if opaqueIDSegment.MatchString(as[n]) && opaqueIDSegment.MatchString(bs[n]) {
			continue
		}
		return false
	}
	return true
}

func queriesMatch(a, b url.Values) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || len(av) != len(bv) {
			return false
		}
		sort.Strings(av)
		sort.Strings(bv)
		for n := range av {
			if av[n] != bv[n] {
				return false
			}
		}
	}
	return true
}

func bodiesMatch(a, b string) bool {
	if a == "" && b == "" {
		return true
	}
	canA, okA := canonicalJSON(a)
	canB, okB := canonicalJSON(b)
	if !okA || !okB {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	return jsonEqualIgnoringRedactedAndProof(canA, canB)
}

func canonicalJSON(s string) (any, bool) {
	if strings.TrimSpace(s) == "" {
		return nil, false
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, false
	}
	return v, true
}

var srpIgnoredKeys = map[string]bool{
	"ClientProof":     true,
	"ClientEphemeral": true,
	"SrpSession":      true,
	"TwoFactorCode":   true,
}

func jsonEqualIgnoringRedactedAndProof(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return false
		}
		keys := map[string]bool{}
		for k := range av {
			keys[k] = true
		}
		for k := range bv {
			keys[k] = true
		}
		for k := range keys {
			if srpIgnoredKeys[k] {
				_, inA := av[k]
				_, inB := bv[k]
				if inA != inB {
					return false
				}
				continue
			}
			if !jsonEqualIgnoringRedactedAndProof(av[k], bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for n := range av {
			if !jsonEqualIgnoringRedactedAndProof(av[n], bv[n]) {
				return false
			}
		}
		return true
	case string:
		bv, ok := b.(string)
		if !ok {
			return false
		}
		if strings.HasPrefix(av, "REDACTED_") || strings.HasPrefix(bv, "REDACTED_") {
			return true
		}
		return av == bv
	default:
		return a == b
	}
}

func readRequestBody(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	r.Body = io.NopCloser(bytes.NewBuffer(b))
	return string(b), nil
}
