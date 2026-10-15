# KikBite web: builds the bundle and serves it with scripts/serve.mjs (security headers, CSP, gzip, /config.js, /healthz,
# and a same-origin proxy of /api/* and /ws to the Go API so the browser needs no CORS).
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install --no-audit --no-fund
COPY . .
RUN node scripts/build.mjs

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=8080
COPY --from=build /app/dist ./dist
COPY --from=build /app/scripts/serve.mjs ./scripts/serve.mjs
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/healthz || exit 1
CMD ["node", "scripts/serve.mjs"]
