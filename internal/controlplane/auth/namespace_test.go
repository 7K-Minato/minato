package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchNamespacePattern(t *testing.T) {
	cases := []struct {
		pattern, ns string
		want        bool
	}{
		{"tenant-a", "tenant-a", true},
		{"tenant-a", "tenant-b", false},
		{"*", "anything", true},
		{"*", "", true},
		{"tenant-*", "tenant-a", true},
		{"tenant-*", "tenant-", true},
		{"tenant-*", "tenant", false},
		{"tenant-*", "other-a", false},
		{"tenant-*", "x-tenant-a", false},
		{"", "tenant-a", false},
		{"tenant-a*", "tenant-ab", true},
		{"*a", "tenant-a", false}, // only trailing wildcards supported
		{"ten*ant", "tenant", false},
	}
	for _, c := range cases {
		if got := MatchNamespacePattern(c.pattern, c.ns); got != c.want {
			t.Errorf("MatchNamespacePattern(%q, %q) = %v, want %v", c.pattern, c.ns, got, c.want)
		}
	}
}

func TestMatchAnyNamespace(t *testing.T) {
	assert.True(t, MatchAnyNamespace([]string{"tenant-a", "prod-*"}, "prod-eu"))
	assert.True(t, MatchAnyNamespace([]string{"tenant-a", "prod-*"}, "tenant-a"))
	assert.False(t, MatchAnyNamespace([]string{"tenant-a", "prod-*"}, "staging"))
	assert.False(t, MatchAnyNamespace(nil, "tenant-a"))
}

func TestParseNamespaces(t *testing.T) {
	assert.Equal(t, []string{"tenant-a", "tenant-*"}, ParseNamespaces("tenant-a, tenant-*"))
	assert.Equal(t, []string{"a"}, ParseNamespaces("a"))
	assert.Nil(t, ParseNamespaces(""))
	assert.Nil(t, ParseNamespaces(" , ,"))
	assert.Equal(t, []string{"a", "b"}, ParseNamespaces("a,,b"))
}

func TestUserAllowsNamespace(t *testing.T) {
	clusterWide := &User{ID: "1", Username: "admin"}
	assert.True(t, clusterWide.ClusterWide())
	assert.True(t, clusterWide.AllowsNamespace("anything"))

	scoped := &User{ID: "2", Username: "ci", Namespaces: []string{"tenant-a", "tenant-*"}}
	assert.False(t, scoped.ClusterWide())
	assert.True(t, scoped.AllowsNamespace("tenant-a"))
	assert.True(t, scoped.AllowsNamespace("tenant-b"))
	assert.False(t, scoped.AllowsNamespace("other"))

	var nilUser *User
	assert.True(t, nilUser.ClusterWide())
	assert.True(t, nilUser.AllowsNamespace("anything"))
}
