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

echo "==> Docker installed and running."
