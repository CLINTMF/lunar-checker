# Lunar Checker

Real Minecraft player stats scraper with a fast Go backend and a dark web frontend.

## Supported servers

- **DonutSMP** — live public scrape through DonutStats, including money, shards, playtime, kills, deaths, K/D, blocks, mobs, and money flow.
- **Hypixel SkyBlock** — adapter endpoint is included and returns a clear configuration response until `HYPIXEL_API_KEY` is added as a server secret.

## Run

```bash
go run .
```

The server defaults to port `3000`:

```bash
PORT=8080 go run .
```

## API

```text
GET /health
GET /api/minecraft/donut/:username
GET /api/minecraft/hypixel/:username
```

Example:

```bash
curl http://localhost:3000/api/minecraft/donut/LoadFc
```

The API validates usernames, fetches the source server-side, caches results for
60 seconds, and never exposes server secrets to the browser.

## GitHub Pages

GitHub Pages can host the static landing page, but it cannot run the Go scraper.
Use the live Go service for the complete interactive checker, or pass its URL to
the frontend with `?api=https://your-api-host.example.com`.