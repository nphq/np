package uiapi

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// 手写校验（非反射），全部返回用户可读的 error，供 errors.go 包装为 Code.

var (
	clusterIDRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)
	jobIDRe     = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,256}$`)
	allocIDRe   = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)
)

// ValidateClusterID 校验集群 ID。
func ValidateClusterID(id string) error {
	if !clusterIDRe.MatchString(id) {
		return fmt.Errorf("invalid cluster ID: %q (allowed: a-z A-Z 0-9 . _ -, max 64)", id)
	}
	return nil
}

// ValidateJobID 校验 job ID。
func ValidateJobID(id string) error {
	if !jobIDRe.MatchString(id) {
		return fmt.Errorf("invalid job ID: %q", id)
	}
	return nil
}

// ValidateAllocID 校验 alloc ID。
func ValidateAllocID(id string) error {
	if !allocIDRe.MatchString(id) {
		return fmt.Errorf("invalid alloc ID: %q", id)
	}
	return nil
}

// ValidateAddress 校验集群地址：必须可解析，scheme 可选（缺省补 http://）。
func ValidateAddress(addr string) (string, error) {
	a := strings.TrimSpace(addr)
	if a == "" {
		return "", fmt.Errorf("address is required")
	}
	if !strings.HasPrefix(a, "http://") && !strings.HasPrefix(a, "https://") {
		a = "http://" + a
	}
	u, err := url.Parse(a)
	if err != nil {
		return "", fmt.Errorf("invalid address %q: %w", addr, err)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("invalid address %q: missing host", addr)
	}
	if port := u.Port(); port != "" && !validPort(port) {
		return "", fmt.Errorf("invalid port in address %q", addr)
	}
	return a, nil
}

func validPort(p string) bool {
	n := 0
	for _, c := range p {
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
		if n > 65535 {
			return false
		}
	}
	return n > 0
}

// ValidateNamespace 校验 namespace 名（可空）。
func ValidateNamespace(ns string) error {
	if ns == "" {
		return nil
	}
	if strings.ContainsAny(ns, " /\\") || len(ns) > 128 {
		return fmt.Errorf("invalid namespace: %q", ns)
	}
	return nil
}

// ValidateRegion 校验 region 名（可空）。
func ValidateRegion(r string) error {
	if r == "" {
		return nil
	}
	if strings.ContainsAny(r, " /\\") || len(r) > 64 {
		return fmt.Errorf("invalid region: %q", r)
	}
	return nil
}

// ValidateClusterName 校验显示名（可空，仅限本地展示）。
func ValidateClusterName(name string) error {
	if len(name) > 64 {
		return fmt.Errorf("name too long (max 64)")
	}
	return nil
}
