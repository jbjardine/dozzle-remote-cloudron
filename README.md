# Remote Log Proxy for Cloudron

[![Release](https://img.shields.io/github/v/release/jbjardine/dozzle-remote-cloudron?sort=semver)](https://github.com/jbjardine/dozzle-remote-cloudron/releases)
[![Cloudron](https://img.shields.io/badge/Cloudron-App-blue)](https://www.cloudron.io/)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Dozzle](https://img.shields.io/badge/Dozzle-v8.14.12-3B82F6)](https://github.com/amir20/dozzle)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

<br/>
If this project is useful to you, you can support its development here:

[![Support](https://img.shields.io/badge/Support-Buy%20Me%20a%20Coffee-1f2937)](https://www.buymeacoffee.com/jbjardine)

## Overview

Remote Log Proxy is a Cloudron-ready package that exposes a secure, SSO‑protected configuration UI for connecting a log viewer to a **remote Docker Engine**. It supports **TCP** or **SSH** remotes and uses a lightweight proxy so you don’t need shell access to the Cloudron host.

## Highlights

- Cloudron‑ready package with built‑in `/config` UI
- SSH tunnel support for `ssh://user@host` (no SSH support needed in the viewer)
- Healthcheck wired to the config UI
- Secrets stored in Cloudron local storage

## How It Works

A tiny Go proxy runs on port `8080` and reverse-proxies Dozzle on `8081`.

- **TCP remote:** Dozzle connects directly to `tcp://host:port`.
- **SSH remote:** the startup script opens a local SSH tunnel to the remote socket and Dozzle connects to `tcp://127.0.0.1:2375`.

## Quick Start

1) Install the app on Cloudron  
2) Open the config page:

```
https://<your-app-domain>/config
```

3) Set **Remote Docker Host** to one of:

```
tcp://10.0.0.10:2375
ssh://user@10.0.0.10
```

4) (SSH only) Paste your private key  
5) Optional: set **Display Name** for a friendly label  
6) Click **Save & Apply**

## Configuration Details

For SSH:

- Paste a private key (OpenSSH format) in the UI.
- The key is stored in `/app/data/.ssh/id_rsa` inside the container.
- Click **Save & Apply** to reload without restarting the Cloudron app.

## Security Notes

- **TCP:** Only use `tcp://` on trusted networks.
- **SSH:** The remote host must expose `/var/run/docker.sock` (default on most Docker installs).
- SSH keys are stored in Cloudron local storage (`/app/data/.ssh`).

The SSH tunnel forwards:

```
127.0.0.1:2375 -> /var/run/docker.sock
```

## Repository Structure

```
.
├── CloudronManifest.json
├── Dockerfile
├── start.sh
├── config-proxy
│   ├── go.mod
│   └── main.go
└── README.md
```

## Build and Deploy (Cloudron)

Install the Cloudron CLI:

```bash
npm install -g cloudron
```

Log in to your Cloudron:

```bash
cloudron login my.cloudron.example
```

Build the image:

```bash
cloudron build
```

Then install or update:

```bash
cloudron install --image <image-tag> --location dozzle.example.com
# or
cloudron update --app dozzle.example.com --image <image-tag>
```

## Release Tarball

Releases are created manually (workflow dispatch) and attach a tarball of the Cloudron package.

From GitHub Actions, run the **release** workflow and set the tag (e.g. `v0.1.0`). The workflow creates a **draft** release with the tarball.
The tarball is the Cloudron package ready to download and install.

## Contributing

PRs are welcome. Keep changes minimal and documented. Run builds via Cloudron CLI when possible.

## License

MIT. See [LICENSE](LICENSE).

## Credits

Built on top of [Dozzle](https://github.com/amir20/dozzle).
