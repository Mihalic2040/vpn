# Railway WireGuard TCP gateway

This service uses `wireproxy`, a userspace WireGuard client, to provide a
private, bidirectional TCP routing table between a Railway network and the
WireGuard network on the VPS.

```text
Railway service -> gateway.railway.internal -> WireGuard -> VPN target
VPN peer -> gateway WireGuard address -> Railway/private/public target
```

Routes are static for the lifetime of the process, but target DNS names are
resolved when connections are opened. Traffic is raw, full-duplex TCP; the
gateway does not inspect application protocols.

## Route configuration

`ROUTES_JSON` is a required, non-empty JSON array. Each route has exactly four
fields:

```json
[
  {
    "direction": "vpn_to_local",
    "listen_port": 8887,
    "target_host": "api.railway.internal",
    "target_port": 8000
  },
  {
    "direction": "local_to_vpn",
    "listen_port": 8893,
    "target_host": "10.8.0.2",
    "target_port": 5566
  }
]
```

### `vpn_to_local`

This exposes a listener on the gateway's WireGuard address and forwards the
connection through the container's normal network:

```text
VPN peer -> 10.8.0.3:8887 -> api.railway.internal:8000
```

The target may be:

- another Railway service such as `api.railway.internal`
- a public DNS name
- an IPv4 or IPv6 address
- `localhost`, but only when the target runs in the gateway container itself

In Railway, `localhost` never refers to another service. Use that service's
`*.railway.internal` name instead.

### `local_to_vpn`

This binds a dual-stack listener on the container and forwards the connection
through WireGuard:

```text
Railway service -> gateway.railway.internal:8893 -> 10.8.0.2:5566
```

The target may be an IPv4 address, IPv6 address, or DNS name. The resolved
target address must be covered by `WG_ALLOWED_IPS`, otherwise wireproxy cannot
select the WireGuard peer for the connection.

Listener ports must be unique within a direction. The same numeric port may be
used once for each direction because the listeners exist on separate network
stacks.

## VPS and peer setup

Keep `server/docker-compose.yml` running on the VPS. Create one dedicated
`wg-easy` peer for this gateway and record:

- client private key
- client address, for example `10.8.0.3/32`
- VPS server public key
- peer preshared key
- VPS hostname and WireGuard UDP port
- allowed VPN prefixes, for example `10.8.0.0/24`

The VPS must know both the gateway peer and each destination peer. Its `wg0` to
`wg0` forwarding rule must remain enabled. Other VPN nodes must route the
gateway address through the VPS; the generated `/24` peer profile does this for
the example network.

## Railway deployment

Railway does not use the Compose file for production orchestration. Deploy the
`client` directory as one service:

1. Set the Railway service root directory to `client` and use the Dockerfile.
2. Configure the WireGuard variables and `ROUTES_JSON` shown below.
3. Keep exactly one replica and use a dedicated WireGuard peer identity.
4. Do not create a public domain or TCP Proxy for this private-only deployment.
5. From sibling Railway services, connect to
   `<gateway-service>.railway.internal:<listen_port>`.

Required variables:

```text
WG_PRIVATE_KEY=<gateway peer private key>
WG_ADDRESS=10.8.0.3/32
WG_MTU=1200
WG_SERVER_PUBLIC_KEY=<VPS WireGuard public key>
WG_PRESHARED_KEY=<gateway peer preshared key>
WG_ENDPOINT=vpn.example.com:51820
WG_ALLOWED_IPS=10.8.0.0/24
WG_PERSISTENT_KEEPALIVE=25
ROUTES_JSON=[{"direction":"vpn_to_local","listen_port":8887,"target_host":"api.railway.internal","target_port":8000},{"direction":"local_to_vpn","listen_port":8893,"target_host":"10.8.0.2","target_port":5566}]
```

Route changes take effect after Railway restarts or redeploys the service.
`WG_MTU` is optional and defaults to `1200` for an IPv4-only interface, which
keeps the encapsulated WireGuard packet at or below a 1280-byte underlay. When
`WG_ADDRESS` contains IPv6, the default and minimum are `1280`.

## Local Linux test

Copy `.env.example` to `.env`, replace all placeholder WireGuard values, and
adjust the route targets. The Compose service uses host networking so every
configured `local_to_vpn` listener is available without duplicating the JSON
ports in Compose:

```sh
docker compose up -d --build
docker compose logs -f
```

With host networking, a `vpn_to_local` target of `localhost:8000` refers to the
Linux host. This differs from Railway, where it refers only to the gateway
container.

Test a `local_to_vpn` route through its listener:

```sh
nc -v 127.0.0.1 8893
```

From a VPN peer, test a `vpn_to_local` route against the gateway's WireGuard
address:

```sh
nc -v 10.8.0.3 8887
```

## Validation and troubleshooting

Startup fails before wireproxy runs when a required value is missing, `WG_MTU`
is outside `576` through `65535` for IPv4 or `1280` through `65535` for IPv6,
or `ROUTES_JSON` contains malformed JSON, unknown fields, invalid ports,
invalid hosts, injected control characters, or duplicate listeners.

The generated configuration is stored at `/tmp/wireproxy.conf` with mode
`0600`, checked using wireproxy's configuration test, and never logged by the
entrypoint.

On the VPS, confirm handshakes and transfer counters:

```sh
sudo wg show
```

A successful gateway handshake does not prove another VPN peer or its target
service is reachable. If connections time out, verify the target listener,
peer routing, `AllowedIPs`, firewall rules, and `PersistentKeepalive` on peers
behind NAT.

## Secret rotation

Real WireGuard credentials were previously committed in `client/.env`. That
file is now ignored and must remain local, but old Git commits still contain
the previous values. Delete and recreate the affected peer in `wg-easy`, then
replace `WG_PRIVATE_KEY` and `WG_PRESHARED_KEY` in Railway and local `.env`
before deploying this version. Git history is intentionally not rewritten.
