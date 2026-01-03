# Release Notes

This release bundles the Cloudron package source as a tarball attachment.

## What’s Included
- Cloudron manifest, Dockerfile, and startup script
- Config proxy UI at `/config` with **Save & Apply** reload
- Remote Docker via **TCP** or **SSH** (tunnel)
- Custom icon and updated README

## How to Use
1. Download the tarball from this release.
2. Extract it locally.
3. Build with Cloudron CLI:
   ```bash
   cloudron build
   ```
4. Install the resulting image:
   ```bash
   cloudron install --image <image-tag> --location <app-domain>
   ```

## Post-Install
Open `/config`, set your remote Docker host, paste the SSH key if needed, then **Save & Apply**.
