package server

import (
	"strconv"
	"strings"
)

// isELBv2LabMuxListenerPath reports whether path is /{prefix}/{account}/{lbName}/{port}[...].
// prefix is "alb" or "nlb".
func isELBv2LabMuxListenerPath(prefix, path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return len(parts) >= 4 && parts[0] == prefix && parts[1] != "" && parts[2] != "" && parts[3] != ""
}

// parseELBv2LabMuxListenerPath parses /{prefix}/{account}/{lbName}/{port}[/{path...}].
func parseELBv2LabMuxListenerPath(prefix, path string) (accountID, lbName string, port int, routePath string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 4 || parts[0] != prefix {
		return "", "", 0, "", false
	}
	accountID = parts[1]
	lbName = parts[2]
	port, err := strconv.Atoi(parts[3])
	if err != nil || port <= 0 || accountID == "" || lbName == "" {
		return "", "", 0, "", false
	}
	if len(parts) == 4 {
		routePath = "/"
	} else {
		routePath = "/" + strings.Join(parts[4:], "/")
	}
	return accountID, lbName, port, routePath, true
}
