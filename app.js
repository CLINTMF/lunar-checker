const $ = (id) => document.getElementById(id);
const form = $("form");
const button = $("button");
const configuredApi = new URLSearchParams(location.search).get("api");
const apiBase = (configuredApi || "https://api.github.com").replace(/\/$/, "");
const esc = (value) => String(value ?? "").replace(/[&<>"']/g, (char) => ({
  "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;"
}[char]));

async function getStats(username) {
  if (configuredApi) {
    const response = await fetch(`${apiBase}/stats/github/${encodeURIComponent(username)}`);
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "API request failed");
    return { data: body.data, cached: body.cached, endpoint: `${apiBase}/stats/github/${encodeURIComponent(username)}` };
  }

  const [profileResponse, reposResponse] = await Promise.all([
    fetch(`https://api.github.com/users/${encodeURIComponent(username)}`),
    fetch(`https://api.github.com/users/${encodeURIComponent(username)}/repos?per_page=100&sort=updated`)
  ]);
  const profile = await profileResponse.json();
  const repos = await reposResponse.json();
  if (!profileResponse.ok) {
    if (profileResponse.status === 404) throw new Error("GitHub user not found");
    if (profileResponse.status === 403) throw new Error("GitHub public API rate limit reached");
    throw new Error(profile.message || "Could not fetch profile");
  }
  if (!reposResponse.ok) throw new Error(repos.message || "Could not fetch repositories");

  const owned = repos.filter((repo) => !repo.fork);
  const languages = {};
  owned.forEach((repo) => { if (repo.language) languages[repo.language] = (languages[repo.language] || 0) + 1; });
  const sorted = [...owned].sort((a, b) => (b.stargazers_count - a.stargazers_count) || (b.forks_count - a.forks_count));
  return {
    cached: false,
    endpoint: `https://api.github.com/users/${encodeURIComponent(username)}`,
    data: {
      platform: "github", username: profile.login, name: profile.name, avatar: profile.avatar_url,
      profileUrl: profile.html_url, bio: profile.bio, company: profile.company, location: profile.location,
      stats: {
        followers: profile.followers, following: profile.following, publicRepos: profile.public_repos,
        publicGists: profile.public_gists, totalStars: owned.reduce((sum, repo) => sum + repo.stargazers_count, 0),
        totalForks: owned.reduce((sum, repo) => sum + repo.forks_count, 0),
        openIssues: owned.reduce((sum, repo) => sum + repo.open_issues_count, 0), ownedRepos: owned.length
      },
      languages,
      topRepos: sorted.slice(0, 10).map((repo) => ({
        name: repo.name, description: repo.description, url: repo.html_url, stars: repo.stargazers_count,
        forks: repo.forks_count, language: repo.language, updatedAt: repo.updated_at
      }))
    }
  };
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const username = $("username").value.trim();
  if (!username) return;
  button.disabled = true;
  $("status").textContent = "Fetching live stats…";
  $("status").className = "status";
  $("profile").classList.remove("show");
  try {
    const result = await getStats(username);
    const data = result.data;
    $("avatar").src = data.avatar;
    $("avatar").alt = data.username;
    $("name").textContent = data.name || data.username;
    $("handle").textContent = `@${data.username}${result.cached ? " · cached" : " · live"}`;
    const values = [
      ["Followers", data.stats.followers], ["Public repos", data.stats.publicRepos],
      ["Total stars", data.stats.totalStars], ["Total forks", data.stats.totalForks],
      ["Following", data.stats.following], ["Owned repos", data.stats.ownedRepos],
      ["Open issues", data.stats.openIssues], ["Gists", data.stats.publicGists]
    ];
    $("stats").innerHTML = values.map(([label, value]) =>
      `<div class="card"><div class="value">${Number(value).toLocaleString()}</div><div class="label">${label}</div></div>`
    ).join("");
    $("repos").innerHTML = data.topRepos.length
      ? data.topRepos.map((repo) => `<article class="repo"><a href="${esc(repo.url)}" target="_blank" rel="noreferrer">${esc(repo.name)}</a><p>${esc(repo.description || "No description")}</p><small>★ ${repo.stars} · ⑂ ${repo.forks}${repo.language ? ` · ${esc(repo.language)}` : ""}</small></article>`).join("")
      : "<div class='repo'>No public repositories found.</div>";
    $("endpoint").textContent = result.endpoint;
    $("profile").classList.add("show");
    $("status").textContent = "";
  } catch (error) {
    $("status").textContent = error.message;
    $("status").className = "status error";
  } finally {
    button.disabled = false;
  }
});

form.requestSubmit();