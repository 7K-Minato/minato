package auth

import "strings"

// MatchNamespacePattern reports whether pattern matches the namespace ns.
// Supported forms: exact match ("tenant-a"), the wildcard "*" (all
// namespaces), and a single trailing "*" prefix wildcard ("tenant-*").
func MatchNamespacePattern(pattern, ns string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(ns, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == ns
}

// MatchAnyNamespace reports whether any pattern matches the namespace ns.
// An empty pattern list matches nothing (callers treat empty as cluster-wide
// before reaching this function).
func MatchAnyNamespace(patterns []string, ns string) bool {
	for _, p := range patterns {
		if MatchNamespacePattern(p, ns) {
			return true
		}
	}
	return false
}

// ParseNamespaces parses a comma-separated namespace pattern list as stored
// in the "namespaces" field of API key Secrets (e.g. "tenant-a,tenant-*").
func ParseNamespaces(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
