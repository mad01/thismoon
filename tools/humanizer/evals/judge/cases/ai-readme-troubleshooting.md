---
id: ai-readme-troubleshooting
label: likely_ai
bucket: ai
split: test
source: generated
license: generated for this eval
generator: claude-sonnet-5-5 via claude -p, 2026-10-09
words: 202
notes: README troubleshooting section with bold problem headers, concrete commands and config keys; reads like real project docs. Prompt was Write the Troubleshooting section of a README for a self-hosted photo backup server called shoebox. Cover three common problems. About 200 words. Output only the section, nothing else.
---
## Troubleshooting

**Uploads fail with "413 Request Entity Too Large"**
A reverse proxy in front of shoebox is probably capping request size. In nginx, raise `client_max_body_size` (for example, `client_max_body_size 2g;`) and reload. If you don't use a proxy, check that `max_upload_mb` in `shoebox.toml` is larger than your biggest video file.

**The web UI loads but thumbnails never appear**
Thumbnail generation runs as a background job. Run `shoebox jobs status` to see whether the worker is running. If the queue is stuck, restart the service with `systemctl restart shoebox`. Also make sure the `libvips` package is installed, because the worker exits quietly without it. The worker log is at `/var/log/shoebox/worker.log`.

**The mobile app can't connect to the server**
First, confirm the server is reachable from the phone's network by opening `https://your-host:8443/health` in a mobile browser. A self-signed certificate is the most common cause. Either install the certificate on the device or switch to a trusted one, such as Let's Encrypt. If you're connecting from outside your home network, confirm the port is forwarded and that `public_url` in `shoebox.toml` matches the address the app uses.

If none of these solve your problem, run `shoebox doctor` and include its output when you open an issue.
