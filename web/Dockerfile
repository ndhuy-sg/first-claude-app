# syntax=docker/dockerfile:1.7
# Mộc POS web (Next.js standalone server). Build context: web/.
# Multi-arch: docker buildx build --platform linux/arm64,linux/amd64 web
#
# API_ORIGIN is read by next.config.ts rewrites() at BUILD time and baked into server.js.
# In production Cloudflare Tunnel sends /api/* straight to the Go API, so this rewrite is
# only a fallback (e.g. when opening http://<vm>:3000 over an SSH tunnel).

ARG NODE_VERSION=22

FROM node:${NODE_VERSION}-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund

FROM node:${NODE_VERSION}-alpine AS build
WORKDIR /app
ARG API_ORIGIN=http://api:8080
ENV API_ORIGIN=${API_ORIGIN} NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build && mkdir -p public

FROM node:${NODE_VERSION}-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0 TZ=Asia/Ho_Chi_Minh
RUN apk add --no-cache tzdata
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public
USER node
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:3000/favicon.ico || exit 1
CMD ["node", "server.js"]
