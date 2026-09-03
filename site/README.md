# The website

`https://dhs-suite.vercel.app` — a single static page, in English, with the Romanian version under
`/ro/`. No build step, no JavaScript, no third-party requests; `vercel.json` sets a strict
Content-Security-Policy. The Open Graph image `og.png` is rendered from `dhs.svg` with Cantarell.

Deploy (from the maintainer's machine, Vercel project `dhs-suite`):

```bash
cd site && vercel deploy --prod --yes
```

Anything under `site/.vercel/` and `site/.env*` is local state and stays out of git.
