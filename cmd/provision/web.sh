#!/bin/bash
# Installs Docker and the docker compose plugin on Debian/Ubuntu.
# Idempotent: safe to run multiple times.

set -euo pipefail

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
	echo "Docker and docker compose plugin are already installed."
	exit 0
fi

echo "==> Installing Docker prerequisites..."
for i in 1 2 3; do
	if sudo apt-get -o DPkg::Lock::Timeout=120 update -qq; then
		break
	fi
	if [ "$i" -eq 3 ]; then
		echo "apt-get update failed after 3 attempts"
		exit 1
	fi
	sleep 10
done
sudo apt-get -o DPkg::Lock::Timeout=120 install -y -qq ca-certificates curl gnupg lsb-release

echo "==> Adding Docker APT repository..."
sudo install -m 0755 -d /etc/apt/keyrings
if [ ! -f /etc/apt/keyrings/docker.gpg ]; then
	curl -fsSL https://download.docker.com/linux/debian/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
fi
c=$(lsb_release -cs)
if [ ! -f /etc/apt/sources.list.d/docker.list ]; then
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian ${c} stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
fi

echo "==> Installing Docker..."
for i in 1 2 3; do
	if sudo apt-get -o DPkg::Lock::Timeout=120 update -qq; then
		break
	fi
	if [ "$i" -eq 3 ]; then
		echo "apt-get update failed after 3 attempts"
		exit 1
	fi
	sleep 10
done
sudo apt-get -o DPkg::Lock::Timeout=120 install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

echo "==> Ensuring Docker is running..."
sudo systemctl start docker || true
sudo systemctl enable docker || true

echo "==> Configuring UFW firewall..."
if ! command -v ufw >/dev/null 2>&1; then
	sudo apt-get -o DPkg::Lock::Timeout=120 install -y -qq ufw
fi
ssh_port=$(sudo grep -E -i "^Port [0-9]+" /etc/ssh/sshd_config | awk '{print $2}' | head -n 1)
if [ -z "$ssh_port" ]; then
	ssh_port="22"
fi
echo "Allowing SSH on port $ssh_port..."
sudo ufw allow "$ssh_port"/tcp
echo "Allowing HTTP (port 80) and HTTPS (port 443)..."
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
echo "Enabling UFW..."
sudo ufw --force enable

echo "==> Docker installed and running."
