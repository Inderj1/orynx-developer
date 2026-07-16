#!/usr/bin/env bash
# EC2 user-data: prepares the box to run the Orynx stack + be the agent runtime.
# Idempotent-ish; runs once as root at first boot.
set -uxo pipefail
export DEBIAN_FRONTEND=noninteractive

# --- Swap: lets a t3.large (8GB) build the Next.js image without OOM ---------
if [ ! -f /swapfile ]; then
  fallocate -l 4G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile
  swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

apt-get update -y
apt-get install -y ca-certificates curl gnupg git rsync unzip jq

# --- Docker + compose plugin ------------------------------------------------
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg
echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" > /etc/apt/sources.list.d/docker.list
apt-get update -y
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
usermod -aG docker ubuntu
systemctl enable --now docker

# --- Node 22 (for the agent CLIs) -------------------------------------------
curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
apt-get install -y nodejs

# --- Google Chrome (real browser for agents) --------------------------------
curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | gpg --dearmor -o /etc/apt/keyrings/google-chrome.gpg
echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/google-chrome.gpg] http://dl.google.com/linux/chrome/deb/ stable main" > /etc/apt/sources.list.d/google-chrome.list
apt-get update -y
apt-get install -y google-chrome-stable || apt-get install -y chromium-browser || true

# --- Agent CLIs: Claude Code (coder) + Codex (reviewer) ---------------------
npm install -g @anthropic-ai/claude-code @openai/codex || true

# --- Caddy reverse proxy: :80 -> frontend :3000 (SG restricts :80 to your IP)-
curl -fsSL https://dl.cloudsmith.io/public/caddy/stable/gpg.key | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
apt-get update -y
apt-get install -y caddy || true
printf ':80 {\n\treverse_proxy 127.0.0.1:3000\n}\n' > /etc/caddy/Caddyfile
systemctl enable caddy || true
systemctl restart caddy || true

# Completion marker deploy.sh waits on.
touch /home/ubuntu/.orynx-bootstrap-done
chown ubuntu:ubuntu /home/ubuntu/.orynx-bootstrap-done
