package web

import (
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/picom-code/ezlm/internal/paths"
)

func decodeJSON(r *http.Request, in any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return &apiError{http.StatusUnsupportedMediaType, "send the request as JSON"}
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	if err := json.NewDecoder(r.Body).Decode(in); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return &apiError{http.StatusRequestEntityTooLarge, "the request is too large"}
		}
		return &apiError{http.StatusBadRequest, "the request body isn't valid JSON"}
	}
	return nil
}

func redactPaths(msg string) string {
	sep := string(os.PathSeparator)
	for _, p := range []string{paths.Home(), userHome()} {
		if p == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, p+sep, "~"+sep)
		msg = strings.ReplaceAll(msg, p, "~")
	}
	return msg
}

func loopbackPeer(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr == ""
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// maxBody caps request bodies. Reports carry an app log, so the limit is
// generous but finite.
const maxBody = 8 << 20

type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func userHome() string {
	if u, err := os.UserHomeDir(); err == nil {
		return u
	}
	return ""
}
