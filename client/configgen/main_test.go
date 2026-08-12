package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateConfigSupportsBothDirectionsAndDNS(t *testing.T) {
	env := validEnvironment()
	env["ROUTES_JSON"] = `[
		{"direction":"vpn_to_local","listen_port":8887,"target_host":"api.railway.internal","target_port":8000},
		{"direction":"local_to_vpn","listen_port":8893,"target_host":"10.8.0.2","target_port":5566},
		{"direction":"local_to_vpn","listen_port":8894,"target_host":"2001:db8::2","target_port":443}
	]`

	config, err := generateConfig(mapLookup(env))
	if err != nil {
		t.Fatalf("generateConfig returned an error: %v", err)
	}

	generated := string(config)
	assertContains(t, generated, "[Interface]\nAddress = 10.8.0.3/32\nMTU = 1280\n")
	assertContains(t, generated, "[TCPServerTunnel]\nListenPort = 8887\nTarget = api.railway.internal:8000")
	assertContains(t, generated, "[TCPClientTunnel]\nBindAddress = [::]:8893\nTarget = 10.8.0.2:5566")
	assertContains(t, generated, "[TCPClientTunnel]\nBindAddress = [::]:8894\nTarget = [2001:db8::2]:443")
	if count := strings.Count(generated, "[TCPClientTunnel]"); count != 2 {
		t.Fatalf("expected 2 client tunnels, got %d", count)
	}
}

func TestGenerateConfigUsesConfiguredMTU(t *testing.T) {
	env := validEnvironment()
	env["WG_MTU"] = "1360"

	config, err := generateConfig(mapLookup(env))
	if err != nil {
		t.Fatalf("generateConfig returned an error: %v", err)
	}
	assertContains(t, string(config), "MTU = 1360")
}

func TestGenerateConfigAllowsSamePortAcrossDirections(t *testing.T) {
	env := validEnvironment()
	env["ROUTES_JSON"] = `[
		{"direction":"vpn_to_local","listen_port":9000,"target_host":"localhost","target_port":8000},
		{"direction":"local_to_vpn","listen_port":9000,"target_host":"10.8.0.2","target_port":8000}
	]`

	if _, err := generateConfig(mapLookup(env)); err != nil {
		t.Fatalf("same port across different directions should be valid: %v", err)
	}
}

func TestGenerateConfigNormalizesWireGuardLists(t *testing.T) {
	env := validEnvironment()
	env["WG_ADDRESS"] = "10.8.0.3/32, fd00::3/128"
	env["WG_ALLOWED_IPS"] = "10.8.0.0/24, fd00::/64 "

	config, err := generateConfig(mapLookup(env))
	if err != nil {
		t.Fatalf("generateConfig returned an error: %v", err)
	}
	assertContains(t, string(config), "Address = 10.8.0.3/32, fd00::3/128")
	assertContains(t, string(config), "AllowedIPs = 10.8.0.0/24, fd00::/64")
}

func TestGenerateConfigRejectsInvalidRoutes(t *testing.T) {
	tests := []struct {
		name   string
		routes string
		want   string
	}{
		{name: "malformed JSON", routes: `[`, want: "valid JSON array"},
		{name: "empty table", routes: `[]`, want: "at least one route"},
		{name: "unknown field", routes: `[{"direction":"local_to_vpn","listen_port":8000,"target_host":"10.8.0.2","target_port":80,"extra":true}]`, want: "valid JSON array"},
		{name: "unknown direction", routes: `[{"direction":"sideways","listen_port":8000,"target_host":"10.8.0.2","target_port":80}]`, want: "invalid direction"},
		{name: "missing direction", routes: `[{"listen_port":8000,"target_host":"10.8.0.2","target_port":80}]`, want: "invalid direction"},
		{name: "zero listen port", routes: `[{"direction":"local_to_vpn","listen_port":0,"target_host":"10.8.0.2","target_port":80}]`, want: "listen_port"},
		{name: "large target port", routes: `[{"direction":"local_to_vpn","listen_port":8000,"target_host":"10.8.0.2","target_port":65536}]`, want: "target_port"},
		{name: "missing target", routes: `[{"direction":"local_to_vpn","listen_port":8000,"target_port":80}]`, want: "target_host"},
		{name: "newline injection", routes: `[{"direction":"local_to_vpn","listen_port":8000,"target_host":"good.example\n[Peer]","target_port":80}]`, want: "target_host"},
		{name: "INI injection", routes: `[{"direction":"local_to_vpn","listen_port":8000,"target_host":"good.example;bad","target_port":80}]`, want: "target_host"},
		{name: "trailing value", routes: `[] {}`, want: "exactly one JSON array"},
		{name: "duplicate local listener", routes: `[{"direction":"local_to_vpn","listen_port":8000,"target_host":"10.8.0.2","target_port":80},{"direction":"local_to_vpn","listen_port":8000,"target_host":"10.8.0.3","target_port":81}]`, want: "duplicates local_to_vpn port 8000"},
		{name: "duplicate VPN listener", routes: `[{"direction":"vpn_to_local","listen_port":8000,"target_host":"one.example","target_port":80},{"direction":"vpn_to_local","listen_port":8000,"target_host":"two.example","target_port":81}]`, want: "duplicates vpn_to_local port 8000"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := validEnvironment()
			env["ROUTES_JSON"] = test.routes
			_, err := generateConfig(mapLookup(env))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestGenerateConfigRejectsInvalidWireGuardValuesWithoutLeakingSecrets(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value string
		want  string
	}{
		{name: "private key", field: "WG_PRIVATE_KEY", value: "secret-invalid-private-key", want: "WG_PRIVATE_KEY"},
		{name: "public key", field: "WG_SERVER_PUBLIC_KEY", value: "secret-invalid-public-key", want: "WG_SERVER_PUBLIC_KEY"},
		{name: "preshared key", field: "WG_PRESHARED_KEY", value: "secret-invalid-preshared-key", want: "WG_PRESHARED_KEY"},
		{name: "address", field: "WG_ADDRESS", value: "not-a-prefix", want: "WG_ADDRESS"},
		{name: "small MTU", field: "WG_MTU", value: "1279", want: "WG_MTU"},
		{name: "large MTU", field: "WG_MTU", value: "65536", want: "WG_MTU"},
		{name: "non-integer MTU", field: "WG_MTU", value: "large", want: "WG_MTU"},
		{name: "endpoint", field: "WG_ENDPOINT", value: "vpn.example.com", want: "WG_ENDPOINT"},
		{name: "allowed IPs", field: "WG_ALLOWED_IPS", value: "10.8.0.0", want: "WG_ALLOWED_IPS"},
		{name: "keepalive", field: "WG_PERSISTENT_KEEPALIVE", value: "65536", want: "WG_PERSISTENT_KEEPALIVE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := validEnvironment()
			env[test.field] = test.value
			_, err := generateConfig(mapLookup(env))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error naming %s, got %v", test.field, err)
			}
			if strings.Contains(err.Error(), test.value) {
				t.Fatalf("error leaked rejected value: %v", err)
			}
		})
	}
}

func TestGenerateConfigRequiresEveryInput(t *testing.T) {
	for _, name := range []string{
		"WG_PRIVATE_KEY",
		"WG_ADDRESS",
		"WG_SERVER_PUBLIC_KEY",
		"WG_PRESHARED_KEY",
		"WG_ENDPOINT",
		"WG_ALLOWED_IPS",
		"ROUTES_JSON",
	} {
		t.Run(name, func(t *testing.T) {
			env := validEnvironment()
			delete(env, name)
			_, err := generateConfig(mapLookup(env))
			if err == nil || err.Error() != name+" is required" {
				t.Fatalf("expected required-variable error, got %v", err)
			}
		})
	}
}

func validEnvironment() map[string]string {
	return map[string]string{
		"WG_PRIVATE_KEY":          testKey(1),
		"WG_ADDRESS":              "10.8.0.3/32",
		"WG_SERVER_PUBLIC_KEY":    testKey(2),
		"WG_PRESHARED_KEY":        testKey(3),
		"WG_ENDPOINT":             "vpn.example.com:51820",
		"WG_ALLOWED_IPS":          "10.8.0.0/24",
		"WG_PERSISTENT_KEEPALIVE": "25",
		"ROUTES_JSON":             `[{"direction":"local_to_vpn","listen_port":8893,"target_host":"10.8.0.2","target_port":5566}]`,
	}
}

func testKey(value byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}

func mapLookup(values map[string]string) getenvFunc {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func assertContains(t *testing.T, value, substring string) {
	t.Helper()
	if !strings.Contains(value, substring) {
		t.Fatalf("expected generated config to contain %q\nconfig:\n%s", substring, value)
	}
}
