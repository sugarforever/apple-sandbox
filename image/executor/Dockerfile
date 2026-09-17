FROM node:22-slim

# node:22-slim ships without a usable CA trust store: outbound TLS to
# api.openai.com fails with "unable to get local issuer certificate"
# without this. Found by actually running the executor against a real
# Agents API session, not by inspection.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g @openai/codex@latest

WORKDIR /workspace

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
