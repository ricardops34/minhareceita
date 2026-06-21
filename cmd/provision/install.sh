#!/bin/bash
# Installs PostgreSQL 18 via apt-get.
# Pulumi's native providers (e.g. AWS RDS, GCP CloudSQL) don't cover bare-metal
# package installation. The Command provider (remote.Command) runs this as the
# transport layer, keeping the declarative resource model intact.

set -euo pipefail

echo "==> Detecting OS..."
. /etc/os-release
if [ "$ID" != "ubuntu" ] && [ "$ID" != "debian" ]; then
	echo "Unsupported OS: $ID"
	exit 1
fi

echo "==> Ensuring sudo is available..."
if ! command -v sudo >/dev/null 2>&1; then
	if [ "$EUID" -eq 0 ]; then
		apt-get update -qq
		apt-get install -y -qq sudo
	else
		echo "sudo is not installed and you are not root. Install sudo first or run as root."
		exit 1
	fi
fi

echo "==> Installing prerequisites..."
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
sudo apt-get -o DPkg::Lock::Timeout=120 install -y -qq wget ca-certificates gnupg lsb-release

echo "==> Adding PostgreSQL APT repository..."
wget -qO - https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo gpg --dearmor -o /usr/share/keyrings/postgresql.gpg
c=$(lsb_release -cs)
echo "deb [signed-by=/usr/share/keyrings/postgresql.gpg] http://apt.postgresql.org/pub/repos/apt ${c}-pgdg main" | sudo tee /etc/apt/sources.list.d/pgdg.list > /dev/null

echo "==> Installing PostgreSQL 18..."
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
sudo apt-get -o DPkg::Lock::Timeout=120 install -y -qq postgresql-18

echo "==> Ensuring PostgreSQL is running..."
sudo pg_ctlcluster 18 main start 2>/dev/null || sudo systemctl start postgresql 2>/dev/null

for i in $(seq 1 60); do
	if sudo -u postgres psql -c '\q' 2>/dev/null; then
		echo "PostgreSQL is ready."
		break
	fi
	if [ "$i" -eq 60 ]; then
		echo "PostgreSQL failed to start after 60s"
		exit 1
	fi
	sleep 1
done

echo "==> Configuring remote access..."
sudo sed -i "s/#listen_addresses = 'localhost'/listen_addresses = '*'/" /etc/postgresql/18/main/postgresql.conf
if ! grep -q "^host\s\+all\s\+etl\s" /etc/postgresql/18/main/pg_hba.conf; then
	echo "host all etl 0.0.0.0/0 scram-sha-256" | sudo tee -a /etc/postgresql/18/main/pg_hba.conf
fi
if ! grep -q "^host\s\+all\s\+web\s" /etc/postgresql/18/main/pg_hba.conf; then
	echo "host all web 0.0.0.0/0 scram-sha-256" | sudo tee -a /etc/postgresql/18/main/pg_hba.conf
fi
sudo systemctl restart postgresql 2>/dev/null || sudo pg_ctlcluster 18 main restart

echo "==> PostgreSQL 18 installed and ready."
