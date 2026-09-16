FROM mcr.microsoft.com/devcontainers/base:bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-venv python3-tk xvfb xauth \
    && rm -rf /var/lib/apt/lists/*
COPY requirements.txt /tmp/slideshow-requirements.txt
RUN python3 -m venv /opt/slideshow-venv \
    && /opt/slideshow-venv/bin/pip install --no-cache-dir -r /tmp/slideshow-requirements.txt
ENV PATH="/opt/slideshow-venv/bin:${PATH}"
WORKDIR /workspaces/memorial/slideshow
