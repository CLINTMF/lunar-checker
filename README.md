# Lunar Checker

Fast GitHub stats API with a lightweight HTML frontend and a Go standard-library backend.

Live checker: https://cnvs_2vgsrdsg2r9c7tpz8ehe82scyn-00-3nkgazfu7nlrp.pike.replit.dev/

## Run

```bash
go run .
```

The port defaults to `3000`. Set `PORT` when the host provides one:

```bash
PORT=8080 go run .
```

## Endpoints

```text
GET /
GET /health
GET /stats/github/:username
GET /github/:username
```

Example:

```bash
curl http://localhost:3000/stats/github/AnnonumusCDev
```

The response includes profile data, followers, following, public repositories, total stars, total forks, open issues, language counts, and the top repositories.

## Notes

- Set `GITHUB_TOKEN` as a server secret for the higher authenticated GitHub API rate limit.
- The in-memory cache is 60 seconds and duplicate profile/repository calls run in parallel.
- GitHub Pages can host the HTML but cannot run the Go server. Use a forwarded Codespaces port or a server host such as Railway for the API.

## GitHub Pages frontend

The `public/` folder is ready for GitHub Pages and `.github/workflows/pages.yml`
deploys it automatically after you push to `main`. Because Pages is static, pass
the URL of your running Go API in the page URL:

```text
https://YOUR-USER.github.io/YOUR-REPO/?api=https://YOUR-GO-API.example.com
```

When the frontend and API are served from the same origin, no `api` parameter is needed.