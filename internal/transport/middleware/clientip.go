package middleware

import (
	"net"
	"net/http"
	"strings"
)

type ClientIPResolver struct {
	trustedProxies []*net.IPNet
}

func NewClientIPResolver(trustedProxies []string) (*ClientIPResolver, error) {
	resolver := &ClientIPResolver{}
	for _, value := range trustedProxies {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		network, err := parseProxyNetwork(value)
		if err != nil {
			return nil, err
		}
		resolver.trustedProxies = append(resolver.trustedProxies, network)
	}
	return resolver, nil
}

func (r *ClientIPResolver) Resolve(req *http.Request) string {
	remoteIP := parseIPFromRemoteAddr(req.RemoteAddr)
	if remoteIP == nil {
		return strings.TrimSpace(req.RemoteAddr)
	}
	if !r.isTrusted(remoteIP) {
		return remoteIP.String()
	}

	if forwarded := r.forwardedFor(req.Header.Get("X-Forwarded-For")); forwarded != "" {
		return forwarded
	}
	if realIP := parseIP(strings.TrimSpace(req.Header.Get("X-Real-IP"))); realIP != nil {
		return realIP.String()
	}
	return remoteIP.String()
}

func ClientIP(req *http.Request) string {
	resolver, _ := NewClientIPResolver(nil)
	return resolver.Resolve(req)
}

func (r *ClientIPResolver) forwardedFor(headerValue string) string {
	parts := strings.Split(headerValue, ",")
	for idx := len(parts) - 1; idx >= 0; idx-- {
		ip := parseIP(strings.TrimSpace(parts[idx]))
		if ip == nil {
			continue
		}
		if !r.isTrusted(ip) {
			return ip.String()
		}
	}
	for _, raw := range parts {
		ip := parseIP(strings.TrimSpace(raw))
		if ip != nil {
			return ip.String()
		}
	}
	return ""
}

func (r *ClientIPResolver) isTrusted(ip net.IP) bool {
	if ip == nil || len(r.trustedProxies) == 0 {
		return false
	}
	for _, network := range r.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func parseProxyNetwork(value string) (*net.IPNet, error) {
	if _, network, err := net.ParseCIDR(value); err == nil {
		return network, nil
	}

	ip := net.ParseIP(value)
	if ip == nil {
		return nil, &net.ParseError{Type: "IP/CIDR", Text: value}
	}
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}, nil
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
}

func parseIPFromRemoteAddr(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil {
		return parseIP(host)
	}
	return parseIP(strings.TrimSpace(remoteAddr))
}

func parseIP(value string) net.IP {
	if value == "" {
		return nil
	}
	return net.ParseIP(value)
}
