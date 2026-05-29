# deploy/nginx/ — host nginx SNI passthrough for the staging edge

## What this is

`stream-lecrm.conf` turns the staging host's shared systemd nginx into a
layer-4 SNI router so the leCRM Caddy edge can own TLS for
`*.lecrm.gbconsult.me` while the co-located apps keep their existing TLS
vhosts. This is **Edge Option B** from `../README.md`.

> **STATUS: STAGED, NOT APPLIED.** Applying this re-points the host's
> public `:443` and briefly affects `tele-claude`, `aaraume`,
> `conversation`, and `drawlk`. Apply only with an explicit go-ahead and
> the rollback below ready. It is the one outward-facing, shared-infra
> change in the staging bring-up.

## Why a stream front (not a plain vhost)

The host nginx already terminates TLS for several `*.gbconsult.me` vhosts
on `0.0.0.0:443`. Caddy must own the `*.lecrm.gbconsult.me` wildcard
(DNS-01) and terminate its own TLS on `127.0.0.1:8443`. Two processes
cannot both bind `:443`, so nginx is demoted to a thin SNI router: it
peeks at the TLS ClientHello SNI (`ssl_preread`, no decryption) and
forwards the raw bytes either to Caddy or back to nginx's own TLS vhosts,
which move off the public `:443` to an internal `127.0.0.1:8444`.

```
client :443 ─► nginx stream (ssl_preread)
                 ├─ *.lecrm.gbconsult.me ─► 127.0.0.1:8443  (Caddy, lecrm-caddy)
                 └─ everything else       ─► 127.0.0.1:8444  (nginx http vhosts)
```

## Cutover runbook

Prereqs: `lecrm-caddy` is up and has issued the wildcard cert (so
`127.0.0.1:8443` answers TLS), and the DNS record for
`*.lecrm.gbconsult.me` resolves to this host.

1. **Confirm the stream module is present.**
   ```
   nginx -V 2>&1 | tr ' ' '\n' | grep -E 'stream'   # expect --with-stream and ssl_preread
   ls /etc/nginx/modules-enabled/ | grep -i stream    # dynamic module, if any
   ```
   Debian's stock `nginx-full`/`nginx` ships `ngx_stream` + `ssl_preread`
   built in. If only `nginx-light` is installed, `apt install nginx-full`
   first (ask before any apt/sudo).

2. **Relocate the existing TLS vhosts off the public :443 to 8444.** In
   every file under `sites-enabled/` (`default`, `aaraume.gbconsult.me`,
   `conversation.gbconsult.me`, `drawlk.gbconsult.me`, `tele-claude`),
   change:
   ```
   listen 443 ssl;            ->  listen 127.0.0.1:8444 ssl;
   listen [::]:443 ssl;       ->  (delete; loopback v4 is enough)
   ```
   Keep `default_server` on exactly one of them (the `default` vhost).
   Leave the `:80` blocks untouched. (HTTP/2: add `http2 on;` if they
   used the old `listen ... http2`.)

3. **Wire the stream block in at nginx top level** (NOT inside `http{}`).
   Append to `/etc/nginx/nginx.conf`, after the closing `}` of the
   `http {}` block:
   ```
   include /etc/nginx/streams-enabled/*.conf;
   ```
   then:
   ```
   sudo mkdir -p /etc/nginx/streams-enabled
   sudo ln -s /home/gui/Projects/leCRM/deploy/nginx/stream-lecrm.conf \
        /etc/nginx/streams-enabled/stream-lecrm.conf
   ```

4. **Test and reload (never restart).**
   ```
   sudo nginx -t          # MUST pass before reload
   sudo systemctl reload nginx
   ```

5. **Verify.**
   ```
   # lecrm subdomain handshakes against Caddy's wildcard cert:
   curl -sS -o /dev/null -w '%{http_code} ssl=%{ssl_verify_result}\n' https://demo.lecrm.gbconsult.me
   # a co-located app still works (regression check):
   curl -sS -o /dev/null -w '%{http_code}\n' https://tele-claude.gbconsult.me
   ```

## Rollback

```
sudo rm /etc/nginx/streams-enabled/stream-lecrm.conf
# revert the listen-directive edits from step 2 (git/backup the files first)
sudo nginx -t && sudo systemctl reload nginx
```
Back up each edited vhost first: `sudo cp <f> <f>.pre-lecrm` so rollback
is a copy-back. All `sudo`/`apt` steps require explicit approval per the
repo's sudo policy.
