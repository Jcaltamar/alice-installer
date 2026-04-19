FROM jrei/systemd-ubuntu:22.04

# Install runtime dependencies the installer bootstrap needs access to.
# Docker itself is NOT installed here — the installer's bootstrap is responsible.
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl \
        ca-certificates \
        sudo \
        iproute2 \
        kmod \
        iptables \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user with passwordless sudo for the test.
# The installer will add this user to the docker group during bootstrap.
RUN useradd -m -s /bin/bash testuser \
    && echo "testuser ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/testuser \
    && chmod 0440 /etc/sudoers.d/testuser

# jrei/systemd-ubuntu already sets CMD to /sbin/init; no override needed.
