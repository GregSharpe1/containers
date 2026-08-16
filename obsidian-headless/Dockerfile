FROM node:22-bookworm-slim

ARG OBSIDIAN_HEADLESS_VERSION=0.0.14

RUN npm install --global --omit=dev "obsidian-headless@${OBSIDIAN_HEADLESS_VERSION}" && \
    npm cache clean --force

USER node

ENTRYPOINT ["ob"]
