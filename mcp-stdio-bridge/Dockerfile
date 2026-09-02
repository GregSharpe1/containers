FROM node:22.16.0-alpine@sha256:41e4389f3d988d2ed55392df4db1420ad048ae53324a8e2b7c6d19508288107e

RUN npm install --global --omit=dev \
      supergateway@3.4.3 \
      slack-mcp-server@1.3.0 \
      argocd-mcp@0.9.0 \
    && npm cache clean --force

USER 1000:1000

ENTRYPOINT ["supergateway"]
