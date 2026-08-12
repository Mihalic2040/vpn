package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"unicode"
)

const (
	directionLocalToVPN = "local_to_vpn"
	directionVPNToLocal = "vpn_to_local"
	defaultWireGuardMTU = 1280
	minimumWireGuardMTU = 1280
	maximumWireGuardMTU = 65535
)

type route struct {
	Direction  string `json:"direction"`
	ListenPort int    `json:"listen_port"`
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
}

type environment struct {
	privateKey          string
	address             string
	mtu                 int
	serverPublicKey     string
	presharedKey        string
	endpoint            string
	allowedIPs          string
	persistentKeepalive int
	routes              []route
}

type getenvFunc func(string) (string, bool)

func main() {
	config, err := generateConfig(os.LookupEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %s\n", err)
		os.Exit(1)
	}

	if _, err := os.Stdout.Write(config); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error: cannot write generated configuration")
		os.Exit(1)
	}
}

func generateConfig(getenv getenvFunc) ([]byte, error) {
	env, err := parseEnvironment(getenv)
	if err != nil {
		return nil, err
	}

	var config bytes.Buffer
	fmt.Fprintln(&config, "[Interface]")
	fmt.Fprintf(&config, "Address = %s\n", env.address)
	fmt.Fprintf(&config, "MTU = %d\n", env.mtu)
	fmt.Fprintf(&config, "PrivateKey = %s\n", env.privateKey)
	fmt.Fprintln(&config)
	fmt.Fprintln(&config, "[Peer]")
	fmt.Fprintf(&config, "PublicKey = %s\n", env.serverPublicKey)
	fmt.Fprintf(&config, "PresharedKey = %s\n", env.presharedKey)
	fmt.Fprintf(&config, "Endpoint = %s\n", env.endpoint)
	fmt.Fprintf(&config, "AllowedIPs = %s\n", env.allowedIPs)
	fmt.Fprintf(&config, "PersistentKeepalive = %d\n", env.persistentKeepalive)

	for _, route := range env.routes {
		fmt.Fprintln(&config)
		switch route.Direction {
		case directionLocalToVPN:
			fmt.Fprintln(&config, "[TCPClientTunnel]")
			fmt.Fprintf(&config, "BindAddress = [::]:%d\n", route.ListenPort)
		case directionVPNToLocal:
			fmt.Fprintln(&config, "[TCPServerTunnel]")
			fmt.Fprintf(&config, "ListenPort = %d\n", route.ListenPort)
		}
		fmt.Fprintf(&config, "Target = %s\n", net.JoinHostPort(route.TargetHost, strconv.Itoa(route.TargetPort)))
	}

	return config.Bytes(), nil
}

func parseEnvironment(getenv getenvFunc) (*environment, error) {
	privateKey, err := required(getenv, "WG_PRIVATE_KEY")
	if err != nil {
		return nil, err
	}
	if err := validateWireGuardKey("WG_PRIVATE_KEY", privateKey); err != nil {
		return nil, err
	}

	addressValue, err := required(getenv, "WG_ADDRESS")
	if err != nil {
		return nil, err
	}
	address, err := validatePrefixes("WG_ADDRESS", addressValue)
	if err != nil {
		return nil, err
	}

	mtu := defaultWireGuardMTU
	if value, ok := getenv("WG_MTU"); ok && strings.TrimSpace(value) != "" {
		mtu, err = parseMTU(value)
		if err != nil {
			return nil, err
		}
	}

	serverPublicKey, err := required(getenv, "WG_SERVER_PUBLIC_KEY")
	if err != nil {
		return nil, err
	}
	if err := validateWireGuardKey("WG_SERVER_PUBLIC_KEY", serverPublicKey); err != nil {
		return nil, err
	}

	presharedKey, err := required(getenv, "WG_PRESHARED_KEY")
	if err != nil {
		return nil, err
	}
	if err := validateWireGuardKey("WG_PRESHARED_KEY", presharedKey); err != nil {
		return nil, err
	}

	endpointValue, err := required(getenv, "WG_ENDPOINT")
	if err != nil {
		return nil, err
	}
	endpoint, err := validateEndpoint("WG_ENDPOINT", endpointValue)
	if err != nil {
		return nil, err
	}

	allowedIPsValue, err := required(getenv, "WG_ALLOWED_IPS")
	if err != nil {
		return nil, err
	}
	allowedIPs, err := validatePrefixes("WG_ALLOWED_IPS", allowedIPsValue)
	if err != nil {
		return nil, err
	}

	keepalive := 25
	if value, ok := getenv("WG_PERSISTENT_KEEPALIVE"); ok && strings.TrimSpace(value) != "" {
		keepalive, err = parsePortLikeValue("WG_PERSISTENT_KEEPALIVE", value, true)
		if err != nil {
			return nil, err
		}
	}

	routesJSON, err := required(getenv, "ROUTES_JSON")
	if err != nil {
		return nil, err
	}
	routes, err := parseRoutes(routesJSON)
	if err != nil {
		return nil, err
	}

	return &environment{
		privateKey:          privateKey,
		address:             address,
		mtu:                 mtu,
		serverPublicKey:     serverPublicKey,
		presharedKey:        presharedKey,
		endpoint:            endpoint,
		allowedIPs:          allowedIPs,
		persistentKeepalive: keepalive,
		routes:              routes,
	}, nil
}

func parseMTU(value string) (int, error) {
	mtu, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || mtu < minimumWireGuardMTU || mtu > maximumWireGuardMTU {
		return 0, fmt.Errorf("WG_MTU must be an integer from %d through %d", minimumWireGuardMTU, maximumWireGuardMTU)
	}
	return mtu, nil
}

func required(getenv getenvFunc, name string) (string, error) {
	value, ok := getenv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return strings.TrimSpace(value), nil
}

func validateWireGuardKey(name, value string) error {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return fmt.Errorf("%s must be a base64-encoded 32-byte WireGuard key", name)
	}
	return nil
}

func validatePrefixes(name, value string) (string, error) {
	parts := strings.Split(value, ",")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return "", fmt.Errorf("%s must not contain an empty prefix", name)
		}
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return "", fmt.Errorf("%s contains an invalid IP prefix", name)
		}
		normalized = append(normalized, prefix.String())
	}
	return strings.Join(normalized, ", "), nil
}

func validateEndpoint(name, value string) (string, error) {
	host, portValue, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil || validateHost(host) != nil {
		return "", fmt.Errorf("%s must use a valid host:port endpoint", name)
	}
	port, err := parsePortLikeValue(name, portValue, false)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func parsePortLikeValue(name, value string, allowZero bool) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	minimum := 1
	if allowZero {
		minimum = 0
	}
	if err != nil || port < minimum || port > 65535 {
		if allowZero {
			return 0, fmt.Errorf("%s must be an integer from 0 through 65535", name)
		}
		return 0, fmt.Errorf("%s must be an integer from 1 through 65535", name)
	}
	return port, nil
}

func parseRoutes(value string) ([]route, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()

	var routes []route
	if err := decoder.Decode(&routes); err != nil {
		return nil, errors.New("ROUTES_JSON must be a valid JSON array of routes")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, errors.New("ROUTES_JSON must contain at least one route")
	}

	listeners := make(map[string]struct{}, len(routes))
	for index := range routes {
		route := &routes[index]
		route.Direction = strings.TrimSpace(route.Direction)
		route.TargetHost = strings.TrimSpace(route.TargetHost)

		if route.Direction != directionLocalToVPN && route.Direction != directionVPNToLocal {
			return nil, fmt.Errorf("ROUTES_JSON route %d has an invalid direction", index+1)
		}
		if route.ListenPort < 1 || route.ListenPort > 65535 {
			return nil, fmt.Errorf("ROUTES_JSON route %d listen_port must be from 1 through 65535", index+1)
		}
		if route.TargetPort < 1 || route.TargetPort > 65535 {
			return nil, fmt.Errorf("ROUTES_JSON route %d target_port must be from 1 through 65535", index+1)
		}
		if err := validateHost(route.TargetHost); err != nil {
			return nil, fmt.Errorf("ROUTES_JSON route %d target_host is invalid", index+1)
		}

		listenerKey := route.Direction + ":" + strconv.Itoa(route.ListenPort)
		if _, exists := listeners[listenerKey]; exists {
			return nil, fmt.Errorf("ROUTES_JSON route %d duplicates %s port %d", index+1, route.Direction, route.ListenPort)
		}
		listeners[listenerKey] = struct{}{}
	}

	return routes, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return errors.New("ROUTES_JSON must contain exactly one JSON array")
}

func validateHost(host string) error {
	if host == "" || strings.TrimSpace(host) != host {
		return errors.New("host is empty or contains surrounding whitespace")
	}
	for _, character := range host {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return errors.New("host contains whitespace or control characters")
		}
	}
	if strings.ContainsAny(host, "[]=#;") {
		return errors.New("host contains configuration delimiters")
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return nil
	}

	dnsName := strings.TrimSuffix(host, ".")
	if dnsName == "" || len(dnsName) > 253 {
		return errors.New("host is not a valid IP address or DNS name")
	}
	for _, label := range strings.Split(dnsName, ".") {
		if len(label) == 0 || len(label) > 63 || !isAlphaNumeric(label[0]) || !isAlphaNumeric(label[len(label)-1]) {
			return errors.New("host is not a valid IP address or DNS name")
		}
		for index := 1; index < len(label)-1; index++ {
			if !isAlphaNumeric(label[index]) && label[index] != '-' {
				return errors.New("host is not a valid IP address or DNS name")
			}
		}
	}
	return nil
}

func isAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}
