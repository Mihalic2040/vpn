# Railway WireGuard TCP gateway

This service uses `wireproxy`, a userspace WireGuard client, to expose one
static TCP forward through Railway:

```text
Railway TCP Proxy -> wireproxy -> WireGuard VPS -> VPN target
```

The TCP connection is full-duplex, so responses from the VPN target return to
the external client over the same connection. The target is configured by
service variables and cannot be selected by a request client.

## VPS setup

Keep `server/docker-compose.yml` running on the VPS. In the `wg-easy` UI,
create a dedicated peer for this Railway service and record its client values:

- client private key
- client address, for example `10.8.0.2/32`
- VPS server public key
- peer preshared key, if enabled in the generated profile
- VPS public hostname and WireGuard port, for example `vpn.example.com:51820`
- allowed IPs, for example `10.8.0.0/24`

The target application must be reachable at the configured VPN address, for
example `10.8.0.3:8080`. The VPS must have both the Railway peer and target
peer configured.

## Railway deployment

Railway does not run this Compose file as production orchestration. Deploy the
`client` directory as one Railway service:

1. Create a service from the repository.
2. Set the service root directory to `client`.
3. Use the detected `Dockerfile`.
4. Add the variables below in Railway. Store `WG_PRIVATE_KEY` as a sealed
   secret.
5. Set `PORT=8080`.
6. Keep the service at one replica.
7. In Networking, create a TCP Proxy targeting internal port `8080`.

Required variables:

```text
WG_PRIVATE_KEY=<Railway peer private key>
WG_ADDRESS=10.8.0.2/32
WG_SERVER_PUBLIC_KEY=<VPS WireGuard public key>
WG_PRESHARED_KEY=<client peer preshared key>
WG_ENDPOINT=vpn.example.com:51820
WG_ALLOWED_IPS=10.8.0.0/24
WG_PERSISTENT_KEEPALIVE=25
TARGET_HOST=10.8.0.3
TARGET_PORT=8080
PORT=8080
```

Railway provides a TCP proxy hostname and external port, such as
`gateway.proxy.rlwy.net:15140`. Connect to that endpoint using the protocol
served by the target application.

Railway TCP Proxy assigns the external port and forwards it to one internal
service port. To expose another independent TCP port, create another Railway
service from this directory with a different `TARGET_HOST`/`TARGET_PORT` and
its own TCP Proxy.

## Local test

Copy `.env.example` to `.env`, replace the placeholder WireGuard values, then
run:

```sh
docker compose up -d --build
```

The local forward listens on `APP_EXPOSE_PORT` and forwards to the configured
VPN target.

## Troubleshooting

Check the Railway or Docker logs first. On the VPS, confirm the peer handshake
and transfer counters with:

```sh
sudo wg show
```

The client is intentionally not a general network bridge and does not support
VPN-originated new connections toward Railway.
